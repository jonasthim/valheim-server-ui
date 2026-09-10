using System;
using System.Collections.Generic;
using System.Threading;

namespace ValheimUI.Agent
{
    internal sealed class CommandResult
    {
        public bool Ok;
        public string Message = "";
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
        public static readonly string[] Names = { "save", "kick", "ban", "unban", "broadcast" };

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

                default:
                    return new CommandResult { Ok = false, Message = "unknown command " + c.Name };
            }
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
