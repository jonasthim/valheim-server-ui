using System;
using System.Collections.Generic;
using System.Reflection;
using HarmonyLib;
using UnityEngine;

namespace ValheimUI.Agent
{
    /// <summary>A map ping a player sent (middle-click on the in-game map).</summary>
    internal sealed class Ping
    {
        public string Name;
        public Vector3 Pos;
        public DateTime At;
    }

    /// <summary>
    /// Recent map pings, kept for a few seconds and reported in the status
    /// snapshot so the manager's map can animate them like the game does.
    /// </summary>
    internal sealed class Pings
    {
        private const double KeepSeconds = 12;
        private readonly object _lock = new object();
        private readonly List<Ping> _items = new List<Ping>();

        public void Add(string name, Vector3 pos)
        {
            lock (_lock)
            {
                Prune();
                _items.Add(new Ping { Name = name ?? "", Pos = pos, At = DateTime.UtcNow });
                while (_items.Count > 32) _items.RemoveAt(0);
            }
        }

        public List<Ping> Snapshot()
        {
            lock (_lock)
            {
                Prune();
                return new List<Ping>(_items);
            }
        }

        private void Prune()
        {
            var cutoff = DateTime.UtcNow.AddSeconds(-KeepSeconds);
            _items.RemoveAll(p => p.At < cutoff);
        }
    }

    /// <summary>
    /// Sees every routed RPC the server handles and records "ChatMessage"
    /// calls of type Ping (3): the package holds the position, the type and
    /// the sender's name first, in every game version so far. Read-only: the
    /// package position is restored and the call proceeds untouched.
    /// </summary>
    [HarmonyPatch]
    internal static class ChatPingPatch
    {
        private const int PingType = 3; // Talker.Type.Ping
        private static readonly int ChatHash = MapObjects.StableHash("ChatMessage");
        private static FieldInfo _hashField;
        private static FieldInfo _paramsField;
        private static bool _resolved;

        private static MethodBase TargetMethod()
        {
            return AccessTools.Method(typeof(ZRoutedRpc), "HandleRoutedRPC");
        }

        private static void Prefix(object __0)
        {
            try
            {
                if (__0 == null) return;
                if (!_resolved)
                {
                    var t = __0.GetType();
                    _hashField = AccessTools.Field(t, "m_methodHash");
                    _paramsField = AccessTools.Field(t, "m_parameters");
                    _resolved = true;
                }
                if (_hashField == null || _paramsField == null) return;
                if (!(_hashField.GetValue(__0) is int hash) || hash != ChatHash) return;
                var pkg = _paramsField.GetValue(__0) as ZPackage;
                if (pkg == null) return;
                int saved = pkg.GetPos();
                try
                {
                    pkg.SetPos(0);
                    var pos = pkg.ReadVector3();
                    int type = pkg.ReadInt();
                    if (type != PingType) return;
                    string name = "";
                    try { name = pkg.ReadString() ?? ""; } catch (Exception) { }
                    AgentPlugin.Pings?.Add(name, pos);
                }
                finally
                {
                    pkg.SetPos(saved);
                }
            }
            catch (Exception)
            {
            }
        }
    }
}
