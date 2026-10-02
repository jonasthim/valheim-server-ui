using System;
using System.Collections.Generic;
using HarmonyLib;
using UnityEngine;

namespace ValheimUI.Gameplay
{
    /// <summary>
    /// Keeps chosen random events (raids) from starting on their own, while
    /// leaving the "event" command and console untouched so the UI and
    /// operators can still start any of them deliberately.
    /// <see cref="RandEventSystem"/> only runs on the server, so this filter
    /// is server-authoritative: nothing needs to run on a client.
    ///
    /// Two call paths pick a random event, so two Harmony patches are
    /// needed (signatures verified against assembly_valheim.dll, see
    /// scratchpad/p0/SIGNATURES.md):
    ///   1. The normal path: <c>UpdateRandomEvent</c> calls the private
    ///      <c>GetPossibleRandomEvents()</c> to build the candidate list,
    ///      then picks one at random. A postfix removes disabled names from
    ///      the candidate list before anything is picked.
    ///   2. "Standalone" events (RandomEvent.m_standaloneInterval &gt; 0) are
    ///      started directly through the private <c>SetRandomEvent</c>,
    ///      bypassing (1). A prefix on <c>SetRandomEvent</c> skips disabled
    ///      names there too — unless the call was forced by the public
    ///      <c>SetRandomEventByName</c> (the UI's "event" command and the
    ///      console), tracked with <see cref="_forced"/>.
    /// </summary>
    internal static class RaidFilter
    {
        private static bool _enabled;
        private static HashSet<string> _disabled = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
        private static string _lastLogged = "";

        // Main-thread only (the game's update loop is single-threaded), so a
        // plain static bool is enough to track "this SetRandomEvent call was
        // forced by SetRandomEventByName".
        private static bool _forced;

        /// <summary>Applies new [Raids] values. Safe to call repeatedly (e.g. on every config change); logs only when the effective state changes.</summary>
        public static void Configure(bool enabled, string disabledCsv)
        {
            var set = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
            if (!string.IsNullOrEmpty(disabledCsv))
            {
                foreach (var raw in disabledCsv.Split(','))
                {
                    var name = raw.Trim();
                    if (name.Length > 0) set.Add(name);
                }
            }

            _enabled = enabled;
            _disabled = set;

            var summary = !enabled
                ? "raid filter off"
                : "raid filter: " + set.Count + " disabled" + (set.Count > 0 ? " (" + string.Join(", ", set) + ")" : "");
            if (summary == _lastLogged) return;
            _lastLogged = summary;
            GameplayPlugin.Log?.LogInfo(summary);
        }

        private static bool IsBlocked(string name)
        {
            return _enabled && name != null && _disabled.Count > 0 && _disabled.Contains(name);
        }

        [HarmonyPatch(typeof(RandEventSystem), "GetPossibleRandomEvents")]
        [HarmonyPostfix]
        private static void GetPossibleRandomEventsPostfix(ref List<KeyValuePair<RandomEvent, Vector3>> __result)
        {
            try
            {
                if (!_enabled || _disabled.Count == 0 || __result == null) return;
                __result.RemoveAll(kv => kv.Key != null && _disabled.Contains(kv.Key.m_name));
            }
            catch (Exception e)
            {
                GameplayPlugin.Log?.LogWarning("raid filter (candidate list) failed: " + e.Message);
            }
        }

        [HarmonyPatch(typeof(RandEventSystem), "SetRandomEvent", new[] { typeof(RandomEvent), typeof(Vector3) })]
        [HarmonyPrefix]
        private static bool SetRandomEventPrefix(RandomEvent ev)
        {
            try
            {
                if (_forced || ev == null || !IsBlocked(ev.m_name)) return true;
                GameplayPlugin.Log?.LogInfo("raid filter: skipped " + ev.m_name);
                return false;
            }
            catch (Exception e)
            {
                GameplayPlugin.Log?.LogWarning("raid filter (set event) failed: " + e.Message);
                return true;
            }
        }

        [HarmonyPatch(typeof(RandEventSystem), nameof(RandEventSystem.SetRandomEventByName), new[] { typeof(string), typeof(Vector3) })]
        [HarmonyPrefix]
        private static void SetRandomEventByNamePrefix()
        {
            _forced = true;
        }

        // Always runs, even if SetRandomEventByName (or SetRandomEvent below
        // it) throws, so _forced never gets stuck true.
        [HarmonyPatch(typeof(RandEventSystem), nameof(RandEventSystem.SetRandomEventByName), new[] { typeof(string), typeof(Vector3) })]
        [HarmonyFinalizer]
        private static Exception SetRandomEventByNameFinalizer(Exception __exception)
        {
            _forced = false;
            return __exception;
        }
    }
}
