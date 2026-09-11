using System;
using System.Collections.Generic;
using System.Globalization;
using System.Text.RegularExpressions;
using System.Threading;
using UnityEngine;

namespace ValheimUI.Agent
{
    internal sealed class CommandResult
    {
        public bool Ok;
        public string Message = "";
        /// <summary>Optional JSON object appended as "data" in the reply.</summary>
        public string DataJson;
    }

    /// <summary>
    /// A command queued by the HTTP thread and executed on the main thread.
    /// </summary>
    internal sealed class PendingCommand
    {
        public string Name;
        public Dictionary<string, string> Args;
        public string Body;
        public readonly ManualResetEventSlim Done = new ManualResetEventSlim(false);
        public CommandResult Result;
    }

    /// <summary>
    /// The control verbs the manager may invoke. Deliberately a fixed set
    /// with explicit arguments rather than a raw console: every verb maps to
    /// a public game API and is audited by the manager.
    /// </summary>
    internal static class Commands
    {
        public static readonly string[] Names = { "save", "kick", "ban", "unban", "broadcast", "time", "say", "setkey", "removekey", "event", "eventstop" };
        private const int MaxTextLength = 200;

        public static CommandResult Execute(PendingCommand c)
        {
            var znet = ZNet.instance;
            if (znet == null || !znet.IsServer())
            {
                return new CommandResult { Ok = false, Message = "server not running" };
            }
            string target;
            switch (c.Name)
            {
                case "save":
                    znet.Save(false);
                    return new CommandResult { Ok = true, Message = "world save requested" };

                case "kick":
                    if (!TryTarget(c, out target)) return MissingTarget();
                    znet.Kick(target);
                    return new CommandResult { Ok = true, Message = "kicked " + target };

                case "ban":
                    if (!TryTarget(c, out target)) return MissingTarget();
                    znet.Ban(target);
                    return new CommandResult { Ok = true, Message = "banned " + target };

                case "unban":
                    if (!TryTarget(c, out target)) return MissingTarget();
                    znet.Unban(target);
                    return new CommandResult { Ok = true, Message = "unbanned " + target };

                case "broadcast":
                {
                    var text = (c.Body ?? "").Trim();
                    if (text.Length == 0 && c.Args != null && c.Args.TryGetValue("message", out var m)) text = m.Trim();
                    if (text.Length == 0) return new CommandResult { Ok = false, Message = "message is required" };
                    if (text.Length > 200) text = text.Substring(0, 200);
                    var type = (int)MessageHud.MessageType.Center;
                    if (c.Args != null && c.Args.TryGetValue("style", out var style) && style == "topleft")
                    {
                        type = (int)MessageHud.MessageType.TopLeft;
                    }
                    // MessageHud registers "ShowMessage" on every client; it is how
                    // the game itself announces raids and sleeping, so it survives
                    // chat protocol changes between versions.
                    ZRoutedRpc.instance.InvokeRoutedRPC(ZRoutedRpc.Everybody, "ShowMessage", type, text);
                    return new CommandResult { Ok = true, Message = "broadcast sent" };
                }

                case "time":
                    return SetTime(znet, c);

                case "say":
                    return Say(c);

                case "setkey":
                case "removekey":
                    return GlobalKey(c, c.Name == "setkey");

                case "event":
                    return StartEvent(znet, c);

                case "eventstop":
                {
                    var rs = RandEventSystem.instance;
                    if (rs == null) return new CommandResult { Ok = false, Message = "event system not running" };
                    rs.ResetRandomEvent();
                    return new CommandResult { Ok = true, Message = "event stopped", DataJson = "{\"event\":null}" };
                }

                default:
                    return new CommandResult { Ok = false, Message = "unknown command " + c.Name };
            }
        }

        private static string Arg(PendingCommand c, string key)
        {
            string v;
            if (c.Args != null && c.Args.TryGetValue(key, out v) && v != null) return v.Trim();
            return "";
        }

        /// <summary>
        /// Moves the world clock forward: to a time of day (fraction), to the
        /// next morning (the game's own skip, what sleeping does) or by a
        /// number of seconds. The server owns net time, so every client follows.
        /// </summary>
        private static CommandResult SetTime(ZNet znet, PendingCommand c)
        {
            var env = EnvMan.instance;
            if (env == null) return new CommandResult { Ok = false, Message = "environment not running" };
            double now = znet.GetTimeSeconds();
            double dayLen = env.m_dayLengthSec;
            if (dayLen <= 0) dayLen = 1800;
            string skip = Arg(c, "skip"), fraction = Arg(c, "fraction"), seconds = Arg(c, "seconds");
            if (skip == "morning")
            {
                env.SkipToMorning();
            }
            else if (fraction.Length > 0)
            {
                double f;
                if (!double.TryParse(fraction, NumberStyles.Float, CultureInfo.InvariantCulture, out f) || f < 0 || f > 1)
                {
                    return new CommandResult { Ok = false, Message = "fraction must be between 0 and 1" };
                }
                double target = Math.Floor(now / dayLen) * dayLen + f * dayLen;
                if (target <= now) target += dayLen;
                znet.SetNetTime(target);
            }
            else if (seconds.Length > 0)
            {
                int n;
                if (!int.TryParse(seconds, NumberStyles.Integer, CultureInfo.InvariantCulture, out n) || n < 1 || n > 86400)
                {
                    return new CommandResult { Ok = false, Message = "seconds must be between 1 and 86400" };
                }
                znet.SetNetTime(now + n);
            }
            else
            {
                return new CommandResult { Ok = false, Message = "fraction, skip=morning or seconds is required" };
            }
            double t = znet.GetTimeSeconds();
            int day = env.GetDay(t);
            double frac = (t % dayLen) / dayLen;
            int hh = (int)(frac * 24), mm = (int)((frac * 24 - hh) * 60);
            var w = new JsonWriter();
            w.BeginObject();
            w.Prop("day", day);
            w.Prop("day_fraction", frac);
            w.Prop("time_seconds", t);
            w.EndObject();
            return new CommandResult
            {
                Ok = true,
                Message = "time set to day " + day.ToString(CultureInfo.InvariantCulture) + ", " + hh.ToString("00") + ":" + mm.ToString("00"),
                DataJson = w.ToString(),
            };
        }

        /// <summary>
        /// Sends a chat shout from the server, through the same "ChatMessage"
        /// routed RPC players use, so it lands in everyone's chat window.
        /// Newer game builds carry the sender as a UserInfo, older ones as a
        /// name string; UserInfo is built by reflection when the type exists.
        /// </summary>
        private static CommandResult Say(PendingCommand c)
        {
            var text = (c.Body ?? "").Trim();
            if (text.Length == 0) text = Arg(c, "message");
            if (text.Length == 0) return new CommandResult { Ok = false, Message = "message is required" };
            if (text.Length > MaxTextLength) text = text.Substring(0, MaxTextLength);
            var name = Arg(c, "name");
            if (name.Length == 0) name = AgentPlugin.ServerName;
            if (name.Length > 32) name = name.Substring(0, 32);
            object sender = MakeUserInfo(name) ?? (object)name;
            var pos = new Vector3(0f, 30f, 0f);
            ZRoutedRpc.instance.InvokeRoutedRPC(ZRoutedRpc.Everybody, "ChatMessage", pos, 2, sender, text);
            return new CommandResult { Ok = true, Message = "said as " + name };
        }

        private static object MakeUserInfo(string name)
        {
            try
            {
                var t = HarmonyLib.AccessTools.TypeByName("UserInfo");
                if (t == null) return null;
                object ui = Activator.CreateInstance(t);
                SetMember(t, ui, "Name", name);
                SetMember(t, ui, "Gamertag", "");
                SetMember(t, ui, "NetworkUserId", "");
                return ui;
            }
            catch (Exception)
            {
                return null;
            }
        }

        private static void SetMember(Type t, object target, string member, string value)
        {
            var f = HarmonyLib.AccessTools.Field(t, member);
            if (f != null && f.FieldType == typeof(string))
            {
                f.SetValue(target, value);
                return;
            }
            var p = HarmonyLib.AccessTools.Property(t, member);
            if (p != null && p.PropertyType == typeof(string) && p.CanWrite) p.SetValue(target, value, null);
        }

        private static readonly Regex KeyPattern = new Regex("^[A-Za-z0-9_]{1,64}$", RegexOptions.CultureInvariant);

        private static CommandResult GlobalKey(PendingCommand c, bool set)
        {
            var zs = ZoneSystem.instance;
            if (zs == null) return new CommandResult { Ok = false, Message = "zone system not running" };
            var key = Arg(c, "key");
            if (!KeyPattern.IsMatch(key)) return new CommandResult { Ok = false, Message = "key must match [A-Za-z0-9_]{1,64}" };
            if (set) zs.SetGlobalKey(key);
            else zs.RemoveGlobalKey(key);
            var w = new JsonWriter();
            w.BeginObject();
            w.Name("global_keys").BeginArray();
            var keys = zs.GetGlobalKeys();
            if (keys != null) foreach (var k in keys) w.Value(k);
            w.EndArray();
            w.EndObject();
            return new CommandResult { Ok = true, Message = (set ? "set global key " : "removed global key ") + key, DataJson = w.ToString() };
        }

        /// <summary>
        /// Starts one of the world's random events (a raid) at a position: the
        /// given x/z, else a connected player's position, else the centre.
        /// The event system is server-authoritative, so clients follow.
        /// </summary>
        private static CommandResult StartEvent(ZNet znet, PendingCommand c)
        {
            var rs = RandEventSystem.instance;
            if (rs == null || rs.m_events == null) return new CommandResult { Ok = false, Message = "event system not running" };
            var name = Arg(c, "name");
            RandomEvent found = null;
            foreach (var ev in rs.m_events)
            {
                if (ev != null && string.Equals(ev.m_name, name, StringComparison.OrdinalIgnoreCase)) { found = ev; break; }
            }
            if (found == null) return new CommandResult { Ok = false, Message = "unknown event; see /v1/catalog" };
            Vector3 pos = Vector3.zero;
            float x, z;
            if (float.TryParse(Arg(c, "x"), NumberStyles.Float, CultureInfo.InvariantCulture, out x) &&
                float.TryParse(Arg(c, "z"), NumberStyles.Float, CultureInfo.InvariantCulture, out z))
            {
                pos = new Vector3(x, 30f, z);
            }
            else
            {
                foreach (var peer in znet.GetPeers())
                {
                    if (peer != null && peer.IsReady() && peer.m_characterID != ZDOID.None)
                    {
                        pos = peer.m_refPos;
                        break;
                    }
                }
            }
            rs.SetRandomEventByName(found.m_name, pos);
            return new CommandResult { Ok = true, Message = "started " + found.m_name, DataJson = "{\"event\":" + GameState.EventJson(rs.GetCurrentRandomEvent()) + "}" };
        }

        private static bool TryTarget(PendingCommand c, out string target)
        {
            target = null;
            if (c.Args == null) return false;
            if (!c.Args.TryGetValue("target", out target)) return false;
            target = target.Trim();
            return target.Length > 0;
        }

        private static CommandResult MissingTarget() => new CommandResult { Ok = false, Message = "target (player name or host id) is required" };
    }
}
