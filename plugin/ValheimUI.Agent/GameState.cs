using System;
using System.Collections.Generic;
using UnityEngine;

namespace ValheimUI.Agent
{
    /// <summary>
    /// One connected player as seen by the server. Positions are the peer
    /// reference positions the server keeps for zone streaming, so they are
    /// available whether or not the player shares their map position; the
    /// manager decides who may see a player that opted out (visible=false).
    /// </summary>
    internal sealed class PlayerSnapshot
    {
        public long Uid;
        public string Name;
        public string Host;
        public string CharacterId;
        public bool Visible;
        public bool HasPosition;
        public Vector3 Position;
    }

    /// <summary>Everything GET /v1/status reports, captured on the main thread.</summary>
    internal sealed class StateSnapshot
    {
        public bool Ready;
        public string WorldName = "";
        public string SeedName = "";
        public int Seed;
        public long WorldUID;
        public int Day;
        public double DayFraction;
        public bool IsNight;
        public string Weather = "";
        public double TimeSeconds;
        public List<string> GlobalKeys = new List<string>();
        public List<PlayerSnapshot> Players = new List<PlayerSnapshot>();
        public DateTime CapturedAt;

        public string ToJson(string agentVersion, string gameVersion, double uptimeSeconds, List<Ping> pings = null)
        {
            var w = new JsonWriter();
            w.BeginObject();
            w.Prop("agent_version", agentVersion);
            w.Prop("game_version", gameVersion);
            w.Prop("uptime_seconds", uptimeSeconds);
            w.Prop("ready", Ready);
            w.Prop("captured_at", CapturedAt.ToUniversalTime().ToString("o"));
            w.Name("world").BeginObject();
            w.Prop("name", WorldName);
            w.Prop("seed_name", SeedName);
            w.Prop("seed", Seed);
            w.Prop("world_uid", WorldUID);
            w.Prop("day", Day);
            w.Prop("day_fraction", DayFraction);
            w.Prop("is_night", IsNight);
            w.Prop("weather", Weather);
            w.Prop("time_seconds", TimeSeconds);
            w.EndObject();
            w.Name("global_keys").BeginArray();
            foreach (var k in GlobalKeys) w.Value(k);
            w.EndArray();
            w.Name("players").BeginArray();
            foreach (var p in Players)
            {
                w.BeginObject();
                w.Prop("uid", p.Uid);
                w.Prop("name", p.Name ?? "");
                w.Prop("host", p.Host ?? "");
                w.Prop("character_id", p.CharacterId ?? "");
                w.Prop("visible", p.Visible);
                if (p.HasPosition)
                {
                    w.Name("position").BeginObject();
                    w.Prop("x", (double)p.Position.x);
                    w.Prop("y", (double)p.Position.y);
                    w.Prop("z", (double)p.Position.z);
                    w.EndObject();
                }
                w.EndObject();
            }
            w.EndArray();
            // Map pings of the last few seconds (Chat "ChatMessage" of type Ping).
            w.Name("pings").BeginArray();
            if (pings != null)
            {
                foreach (var p in pings)
                {
                    w.BeginObject();
                    w.Prop("name", p.Name ?? "");
                    w.Name("position").BeginObject();
                    w.Prop("x", (double)p.Pos.x);
                    w.Prop("y", (double)p.Pos.y);
                    w.Prop("z", (double)p.Pos.z);
                    w.EndObject();
                    w.Prop("at", p.At.ToUniversalTime().ToString("o"));
                    w.EndObject();
                }
            }
            w.EndArray();
            w.EndObject();
            return w.ToString();
        }
    }

    /// <summary>
    /// Reads the game's state. Every method here must run on the Unity main
    /// thread; the plugin calls Capture from Update and hands the immutable
    /// result to the HTTP thread.
    /// </summary>
    internal static class GameState
    {
        public static StateSnapshot Capture()
        {
            var s = new StateSnapshot { CapturedAt = DateTime.UtcNow };
            var znet = ZNet.instance;
            if (znet == null || !znet.IsServer())
            {
                return s;
            }
            s.Ready = true;

            s.WorldName = znet.GetWorldName() ?? "";
            s.WorldUID = znet.GetWorldUID();
            var gen = WorldGenerator.instance;
            if (gen != null)
            {
                s.Seed = gen.GetSeed();
            }
            s.TimeSeconds = znet.GetTimeSeconds();

            var env = EnvMan.instance;
            if (env != null)
            {
                s.Day = env.GetDay(s.TimeSeconds);
                s.DayFraction = env.GetDayFraction();
                s.IsNight = EnvMan.IsNight();
                var cur = env.GetCurrentEnvironment();
                if (cur != null) s.Weather = cur.m_name ?? "";
            }

            var zs = ZoneSystem.instance;
            if (zs != null)
            {
                var keys = zs.GetGlobalKeys();
                if (keys != null) s.GlobalKeys.AddRange(keys);
            }

            foreach (var peer in znet.GetPeers())
            {
                if (peer == null || !peer.IsReady()) continue;
                var p = new PlayerSnapshot
                {
                    Uid = peer.m_uid,
                    Name = peer.m_playerName ?? "",
                    Host = peer.m_socket != null ? (peer.m_socket.GetHostName() ?? "") : "",
                    CharacterId = peer.m_characterID.ToString(),
                    Visible = peer.m_publicRefPos,
                    HasPosition = peer.m_characterID != ZDOID.None,
                    Position = peer.m_refPos,
                };
                s.Players.Add(p);
            }
            return s;
        }

        public static string GameVersion()
        {
            try { return global::Version.GetVersionString(); }
            catch (Exception) { return ""; }
        }
    }
}
