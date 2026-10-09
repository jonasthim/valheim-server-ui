using System;
using System.Reflection;
using BepInEx;
using BepInEx.Logging;
using HarmonyLib;

namespace ValheimUI.SurvivalClient
{
    [BepInPlugin("se.jonasthim.valheimui.survivalclient", "Valheim UI Survival Client", BuildInfo.Version)]
    public sealed class SurvivalClientPlugin : BaseUnityPlugin
    {
        internal static ManualLogSource Log { get; private set; }

        private void Awake()
        {
            Log = Logger;
            try { new Harmony("se.jonasthim.valheimui.survivalclient").PatchAll(typeof(SurvivalClientPlugin).Assembly); }
            catch (Exception e) { Logger.LogWarning("Survival Client death patch failed: " + e.Message); }
        }
    }

    [HarmonyPatch(typeof(Player), "OnDeath")]
    internal static class DeathDetailsPatch
    {
        private static readonly FieldInfo LastHit = AccessTools.Field(typeof(Character), "m_lastHit");

        private static void Prefix(Player __instance)
        {
            try
            {
                if (__instance == null || __instance != Player.m_localPlayer || LastHit == null) return;
                var zdo = __instance.GetComponent<ZNetView>()?.GetZDO();
                if (zdo == null) return;

                var hit = LastHit.GetValue(__instance) as HitData;
                if (hit == null) return;
                string enemy = "";
                int level = 0;
                try
                {
                    var attacker = hit.GetAttacker();
                    if (attacker != null)
                    {
                        enemy = attacker is Player other ? other.GetPlayerName() : attacker.GetHoverName();
                        level = attacker is Player ? 0 : attacker.GetLevel();
                    }
                }
                catch (Exception) { }
                enemy = (enemy ?? "").Trim();
                if (enemy.Length > 120) enemy = enemy.Substring(0, 120);
                zdo.Set(SurvivalDeathKeys.Enemy, enemy);
                zdo.Set(SurvivalDeathKeys.EnemyLevel, level);
                zdo.Set(SurvivalDeathKeys.Situation,
                    hit.m_hitType == HitData.HitType.Undefined ? "" : hit.m_hitType.ToString());
            }
            catch (Exception e) { SurvivalClientPlugin.Log?.LogWarning("death details capture failed: " + e.Message); }
        }
    }
}
