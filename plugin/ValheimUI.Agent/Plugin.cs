using System;
using System.Collections.Concurrent;
using System.Collections.Generic;
using System.Globalization;
using BepInEx;
using BepInEx.Configuration;
using UnityEngine;
using UnityEngine.Rendering;

namespace ValheimUI.Agent
{
    /// <summary>
    /// Valheim UI Agent: a server-only plugin that exposes the running world
    /// to the manager over a loopback HTTP API (players and positions, day and
    /// weather, global keys, join/leave events) and executes a fixed set of
    /// control commands (save, kick, ban, unban, broadcast). Installed and
    /// configured by the manager together with BepInEx; players need nothing.
    /// </summary>
    [BepInPlugin(Guid, Name, BuildInfo.Version)]
    public sealed class AgentPlugin : BaseUnityPlugin
    {
        public const string Guid = "se.jonasthim.valheimui.agent";
        public const string Name = "Valheim UI Agent";

        private const int EventBuffer = 500;

        private ConfigEntry<int> _port;
        private ConfigEntry<string> _bind;
        private ConfigEntry<string> _token;
        private ConfigEntry<int> _intervalMs;

        private HttpApi _api;
        private readonly ConcurrentQueue<PendingCommand> _commands = new ConcurrentQueue<PendingCommand>();
        private volatile string _statusJson = "{\"ready\":false}";
        private float _lastCapture;
        private float _startedAt;
        private string _gameVersion = "";

        private readonly object _eventsLock = new object();
        private readonly LinkedList<KeyValuePair<long, string>> _events = new LinkedList<KeyValuePair<long, string>>();
        private long _eventSeq;
        private Dictionary<long, string> _lastPeers = new Dictionary<long, string>();

        private void Awake()
        {
            _port = Config.Bind("Server", "Port", 0, "TCP port of the loopback API. 0 uses the game's -port value (TCP, so it never collides with the game's UDP ports).");
            _bind = Config.Bind("Server", "BindAddress", "127.0.0.1", "Address to listen on. Keep it on loopback; the manager runs on the same host.");
            _token = Config.Bind("Server", "Token", "", "Bearer token the manager must present. Written by Valheim Server UI before each start; requests are refused while empty.");
            _intervalMs = Config.Bind("Server", "SnapshotIntervalMs", 500, "How often the world state is captured for GET /v1/status.");

            if (SystemInfo.graphicsDeviceType != GraphicsDeviceType.Null)
            {
                Logger.LogInfo("Valheim UI Agent is a dedicated-server plugin; not starting on a game client.");
                return;
            }

            var port = _port.Value > 0 ? _port.Value : GamePortFromCommandLine();
            _startedAt = Time.realtimeSinceStartup;
            _api = new HttpApi(Logger,
                () => _statusJson,
                EventsSince,
                c => _commands.Enqueue(c),
                () => _token.Value);
            try
            {
                _api.Start(_bind.Value, port);
            }
            catch (Exception e)
            {
                Logger.LogError("Valheim UI Agent could not listen on " + _bind.Value + ":" + port + ": " + e.Message);
                _api = null;
            }
            if (string.IsNullOrEmpty(_token.Value))
            {
                Logger.LogWarning("Valheim UI Agent has no token configured; every request will be refused until the manager writes one.");
            }
        }

        private void OnDestroy()
        {
            _api?.Stop();
        }

        private void Update()
        {
            if (_api == null) return;

            var now = Time.realtimeSinceStartup;
            if ((now - _lastCapture) * 1000f >= Math.Max(100, _intervalMs.Value))
            {
                _lastCapture = now;
                try
                {
                    var snap = GameState.Capture();
                    if (snap.Ready && _gameVersion.Length == 0) _gameVersion = GameState.GameVersion();
                    _statusJson = snap.ToJson(BuildInfo.Version, _gameVersion, now - _startedAt);
                    DiffPeers(snap);
                }
                catch (Exception e)
                {
                    Logger.LogWarning("agent snapshot failed: " + e.Message);
                }
            }

            while (_commands.TryDequeue(out var cmd))
            {
                try { cmd.Result = Commands.Execute(cmd); }
                catch (Exception e) { cmd.Result = new CommandResult { Ok = false, Message = e.Message }; }
                finally { cmd.Done.Set(); }
                if (cmd.Result != null && cmd.Result.Ok)
                {
                    PushEvent("command", "{\"name\":" + JsonWriter.Quote(cmd.Name) + ",\"message\":" + JsonWriter.Quote(cmd.Result.Message) + "}");
                }
            }
        }

        private void DiffPeers(StateSnapshot snap)
        {
            var current = new Dictionary<long, string>();
            foreach (var p in snap.Players) current[p.Uid] = p.Name;
            foreach (var kv in current)
            {
                if (!_lastPeers.ContainsKey(kv.Key))
                {
                    PushEvent("player.join", "{\"uid\":" + kv.Key.ToString(CultureInfo.InvariantCulture) + ",\"name\":" + JsonWriter.Quote(kv.Value) + "}");
                }
            }
            foreach (var kv in _lastPeers)
            {
                if (!current.ContainsKey(kv.Key))
                {
                    PushEvent("player.leave", "{\"uid\":" + kv.Key.ToString(CultureInfo.InvariantCulture) + ",\"name\":" + JsonWriter.Quote(kv.Value) + "}");
                }
            }
            _lastPeers = current;
        }

        private void PushEvent(string kind, string dataJson)
        {
            lock (_eventsLock)
            {
                _eventSeq++;
                var json = "{\"seq\":" + _eventSeq.ToString(CultureInfo.InvariantCulture)
                           + ",\"kind\":" + JsonWriter.Quote(kind)
                           + ",\"at\":" + JsonWriter.Quote(DateTime.UtcNow.ToString("o"))
                           + ",\"data\":" + dataJson + "}";
                _events.AddLast(new KeyValuePair<long, string>(_eventSeq, json));
                while (_events.Count > EventBuffer) _events.RemoveFirst();
            }
        }

        private string EventsSince(long since)
        {
            var w = new System.Text.StringBuilder(1024);
            lock (_eventsLock)
            {
                w.Append("{\"next\":").Append(_eventSeq.ToString(CultureInfo.InvariantCulture)).Append(",\"events\":[");
                bool first = true;
                foreach (var kv in _events)
                {
                    if (kv.Key <= since) continue;
                    if (!first) w.Append(',');
                    first = false;
                    w.Append(kv.Value);
                }
                w.Append("]}");
            }
            return w.ToString();
        }

        private static int GamePortFromCommandLine()
        {
            var args = Environment.GetCommandLineArgs();
            for (int i = 0; i + 1 < args.Length; i++)
            {
                if (string.Equals(args[i], "-port", StringComparison.OrdinalIgnoreCase)
                    && int.TryParse(args[i + 1], NumberStyles.Integer, CultureInfo.InvariantCulture, out var p)
                    && p > 0 && p < 65536)
                {
                    return p;
                }
            }
            return 2456;
        }
    }
}
