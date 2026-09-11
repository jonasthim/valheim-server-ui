using System;
using System.Collections.Generic;
using System.Globalization;
using UnityEngine;

namespace ValheimUI.Agent
{
    /// <summary>One line of in-game chat the server relayed.</summary>
    internal sealed class ChatLine
    {
        public long Seq;
        public DateTime At;
        public string Type; // shout | normal
        public string Sender;
        public string Text;
        public Vector3 Pos;
    }

    /// <summary>
    /// Recent chat, recorded from the "ChatMessage" routed RPCs every client
    /// sends through the server. Shouts and normal chat only: whispers are
    /// never kept. Memory only, newest 500 lines.
    /// </summary>
    internal sealed class ChatLog
    {
        private const int Capacity = 500;
        private readonly object _lock = new object();
        private readonly LinkedList<ChatLine> _items = new LinkedList<ChatLine>();
        private long _seq;

        public void Add(string type, string sender, string text, Vector3 pos)
        {
            if (string.IsNullOrEmpty(text)) return;
            if (text.Length > 512) text = text.Substring(0, 512);
            lock (_lock)
            {
                _seq++;
                _items.AddLast(new ChatLine { Seq = _seq, At = DateTime.UtcNow, Type = type, Sender = sender ?? "", Text = text, Pos = pos });
                while (_items.Count > Capacity) _items.RemoveFirst();
            }
        }

        /// <summary>Lines after seq, oldest first, at most limit; "next" is the
        /// sequence to poll from (the last line returned, or the newest known).</summary>
        public string Since(long since, int limit)
        {
            if (limit <= 0 || limit > 200) limit = 200;
            var w = new JsonWriter();
            w.BeginObject();
            w.Name("messages").BeginArray();
            long next;
            lock (_lock)
            {
                next = _seq;
                int n = 0;
                foreach (var m in _items)
                {
                    if (m.Seq <= since) continue;
                    if (n >= limit)
                    {
                        next = m.Seq - 1;
                        break;
                    }
                    n++;
                    w.BeginObject();
                    w.Prop("seq", m.Seq);
                    w.Prop("at", m.At.ToString("o", CultureInfo.InvariantCulture));
                    w.Prop("type", m.Type);
                    w.Prop("sender", m.Sender);
                    w.Prop("text", m.Text);
                    w.Name("position").BeginObject();
                    w.Prop("x", (double)m.Pos.x);
                    w.Prop("y", (double)m.Pos.y);
                    w.Prop("z", (double)m.Pos.z);
                    w.EndObject();
                    w.EndObject();
                }
            }
            w.EndArray();
            w.Prop("next", next);
            w.EndObject();
            return w.ToString();
        }
    }

    /// <summary>
    /// What the manager offers in its pickers: known global keys and the
    /// world's random events. Built on the main thread once the world is
    /// ready and refreshed now and then.
    /// </summary>
    internal static class Catalog
    {
        private static readonly string[] KnownKeys =
        {
            "defeated_eikthyr", "defeated_gdking", "defeated_bonemass", "defeated_dragon", "defeated_goblinking",
            "defeated_queen", "defeated_fader", "defeated_hive", "KilledTroll", "killed_surtling", "KilledBat",
            "Hildir1", "Hildir2", "Hildir3", "nomap", "noportals",
        };

        public static string Build(string serverName)
        {
            var keys = new SortedSet<string>(StringComparer.Ordinal);
            foreach (var k in KnownKeys) keys.Add(k);
            try
            {
                var t = HarmonyLib.AccessTools.TypeByName("GlobalKeys");
                if (t != null && t.IsEnum)
                {
                    foreach (var name in Enum.GetNames(t))
                    {
                        if (name != "None" && name != "Max") keys.Add(name);
                    }
                }
            }
            catch (Exception)
            {
            }
            var w = new JsonWriter();
            w.BeginObject();
            w.Name("global_keys").BeginArray();
            foreach (var k in keys) w.Value(k);
            w.EndArray();
            w.Name("events").BeginArray();
            try
            {
                var rs = RandEventSystem.instance;
                if (rs != null && rs.m_events != null)
                {
                    foreach (var ev in rs.m_events)
                    {
                        if (ev == null || string.IsNullOrEmpty(ev.m_name)) continue;
                        w.BeginObject();
                        w.Prop("name", ev.m_name);
                        w.Prop("duration_seconds", (double)ev.m_duration);
                        w.EndObject();
                    }
                }
            }
            catch (Exception)
            {
            }
            w.EndArray();
            w.Prop("server_name", serverName ?? "Server");
            w.EndObject();
            return w.ToString();
        }
    }
}
