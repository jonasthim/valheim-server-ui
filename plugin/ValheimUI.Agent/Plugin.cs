using System;
using System.Collections.Concurrent;
using System.Collections.Generic;
using System.Globalization;
using System.IO;
using System.Text;
using BepInEx;
using HarmonyLib;
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
        private ConfigEntry<int> _mapResolution;
        private ConfigEntry<int> _mapBudgetMs;
        private ConfigEntry<bool> _mapAutoRender;

        private readonly MapRenderer _map = new MapRenderer();
        private readonly Exploration _explored = new Exploration();
        private readonly Discoveries _discoveries = new Discoveries();
        private readonly Pings _pings = new Pings();
        private readonly ChatLog _chat = new ChatLog();
        private readonly MapObjects _objects;
        private ConfigEntry<string> _serverName;
        private volatile string _catalogJson = "{\"global_keys\":[],\"events\":[],\"server_name\":\"Server\"}";
        private float _lastCatalog = -1f;

        /// <summary>For the Harmony patch that records Vegvisir discoveries.</summary>
        internal static Discoveries Discoveries { get; private set; }
        /// <summary>For the Harmony patch that records map pings.</summary>
        internal static Pings Pings { get; private set; }
        /// <summary>For the Harmony patch that records chat.</summary>
        internal static ChatLog Chat { get; private set; }
        internal static AgentPlugin Instance { get; private set; }
        private static ConfigEntry<string> _serverNameOverride;

        /// <summary>
        /// Name the "say" command speaks as and the catalog reports: the
        /// [Chat] ServerName override when set, else the game's own server
        /// name (the -name launch argument, ZNet.m_ServerName), else "Server".
        /// Resolved on each call because the game fills the static after start-up.
        /// </summary>
        internal static string ResolveServerName()
        {
            var o = _serverNameOverride?.Value;
            if (!string.IsNullOrEmpty(o)) return o.Trim();
            try
            {
                var f = HarmonyLib.AccessTools.Field(typeof(ZNet), "m_ServerName");
                var v = f != null && f.IsStatic ? f.GetValue(null) as string : null;
                if (string.IsNullOrEmpty(v))
                {
                    var p = HarmonyLib.AccessTools.Property(typeof(ZNet), "m_ServerName") ?? HarmonyLib.AccessTools.Property(typeof(ZNet), "ServerName");
                    if (p != null && p.GetGetMethod(true) != null && p.GetGetMethod(true).IsStatic) v = p.GetValue(null, null) as string;
                }
                if (!string.IsNullOrEmpty(v)) return v.Trim();
            }
            catch (Exception)
            {
            }
            return "Server";
        }
        internal static BepInEx.Logging.ManualLogSource Log { get; private set; }
        private float _worldReadyAt = -1f;
        private int _worldSeed;
        private string _cacheDir = "";

        private HttpApi _api;
        private readonly ConcurrentQueue<PendingCommand> _commands = new ConcurrentQueue<PendingCommand>();
        private volatile string _statusJson = "{\"ready\":false}";
        private float _lastCapture;
        private float _startedAt;
        private string _gameVersion = "";
        private StateSnapshot _latestSnapshot;
        private readonly string _runId = System.Guid.NewGuid().ToString("N");

        private readonly object _eventsLock = new object();
        private readonly LinkedList<KeyValuePair<long, string>> _events = new LinkedList<KeyValuePair<long, string>>();
        private readonly List<KeyValuePair<long, string>> _deathEvents = new List<KeyValuePair<long, string>>();
        private readonly HashSet<ZDOID> _recordedDeaths = new HashSet<ZDOID>();
        private string _deathJournalPath = "";
        private long _eventSeq;
        private Dictionary<long, string> _lastPeers = new Dictionary<long, string>();

        public AgentPlugin()
        {
            _objects = new MapObjects(_explored, _discoveries);
            Discoveries = _discoveries;
            Pings = _pings;
            Chat = _chat;
        }

        private void Awake()
        {
            _port = Config.Bind("Server", "Port", 0, "TCP port of the loopback API. 0 uses the game's -port value (TCP, so it never collides with the game's UDP ports).");
            _bind = Config.Bind("Server", "BindAddress", "127.0.0.1", "Address to listen on. Keep it on loopback; the manager runs on the same host.");
            _token = Config.Bind("Server", "Token", "", "Bearer token the manager must present. Written by Valheim Server UI before each start; requests are refused while empty.");
            _intervalMs = Config.Bind("Server", "SnapshotIntervalMs", 500, "How often the world state is captured for GET /v1/status.");
            _mapResolution = Config.Bind("Map", "Resolution", 2048, "Side length in cells of the sampled world map layers (256-4096). 2048 matches the game's own map; 4096 gives the sharpest coastlines at deep zoom and takes four times longer to sample once per world.");
            _mapBudgetMs = Config.Bind("Map", "RenderBudgetMs", 4, "Milliseconds per server frame spent rendering the map. Lower values render slower but never stall the game.");
            _mapAutoRender = Config.Bind("Map", "AutoRender", true, "Render the map shortly after the world has loaded instead of on first request.");
            _serverName = Config.Bind("Chat", "ServerName", "", "Name shown as the sender when the manager sends a chat message (the say command). Empty uses the game's server name (-name).");
            _serverNameOverride = _serverName;
            _cacheDir = System.IO.Path.Combine(Paths.CachePath, "valheimui-agent");

            if (SystemInfo.graphicsDeviceType != GraphicsDeviceType.Null)
            {
                Logger.LogInfo("Valheim UI Agent is a dedicated-server plugin; not starting on a game client.");
                return;
            }
            Log = Logger;
            Instance = this;
            _deathJournalPath = Path.Combine(Paths.ConfigPath, "valheimui-agent", "survival-events.jsonl");
            LoadDeathJournal();
            // New runs start above the previous event cursor even if the manager
            // was offline too long to detect the agent uptime resetting.
            _eventSeq = Math.Max(_eventSeq, DateTime.UtcNow.Ticks);
            try
            {
                new Harmony("se.jonasthim.valheimui.agent").PatchAll(typeof(AgentPlugin).Assembly);
            }
            catch (Exception e)
            {
                Logger.LogWarning("Vegvisir discoveries will not be tracked (patch failed): " + e.Message);
            }

            var port = _port.Value > 0 ? _port.Value : GamePortFromCommandLine();
            _startedAt = Time.realtimeSinceStartup;
            _api = new HttpApi(Logger,
                () => _statusJson,
                EventsSince,
                c => _commands.Enqueue(c),
                () => _token.Value,
                () => _map.InfoJson(),
                () => _map.Png(),
                () => _objects.Json,
                () => _explored.InfoJson(),
                () => _explored.MaskPng(),
                (since, limit) => _chat.Since(since, limit),
                () => _catalogJson);
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
            Instance = null;
            _explored.MaybeSave(true);
            _discoveries.MaybeSave(true);
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
                    _latestSnapshot = snap;
                    if (snap.Ready && _gameVersion.Length == 0) _gameVersion = GameState.GameVersion();
                    if (snap.Ready && _worldReadyAt < 0f)
                    {
                        _worldReadyAt = now;
                        _worldSeed = snap.Seed;
                        _explored.Load(snap.Seed, _cacheDir);
                        _discoveries.Load(snap.Seed, _cacheDir);
                    }
                    if (snap.Ready && _explored.Loaded)
                    {
                        foreach (var p in snap.Players)
                        {
                            if (p.HasPosition) _explored.Explore(p.Position, Exploration.ExploreRadius);
                        }
                        _explored.MaybeSave(false);
                        _explored.MaybeEncode();
                        _discoveries.MaybeSave(false);
                    }
                    _statusJson = snap.ToJson(BuildInfo.Version, _gameVersion, now - _startedAt, _pings.Snapshot());
                    if (snap.Ready) RecordDeathsFromZDOs(snap);
                    DiffPeers(snap);
                }
                catch (Exception e)
                {
                    Logger.LogWarning("agent snapshot failed: " + e.Message);
                }
            }

            if (_worldReadyAt >= 0f)
            {
                // Map: start automatically a little after the world loaded (so
                // the first minutes go to players), then render in slices.
                if (_mapAutoRender.Value && _map.Current == MapRenderer.State.Idle && now - _worldReadyAt > 15f)
                {
                    _map.Begin(_worldSeed, _mapResolution.Value, _cacheDir, false);
                }
                _map.Step(Math.Max(1, _mapBudgetMs.Value));
                _objects.Step();
                if (now - _lastCatalog > 30f)
                {
                    _lastCatalog = now;
                    try { _catalogJson = Catalog.Build(ResolveServerName()); }
                    catch (Exception e) { Logger.LogWarning("agent catalog failed: " + e.Message); }
                }
            }

            while (_commands.TryDequeue(out var cmd))
            {
                if (cmd.Name == "map.render")
                {
                    try
                    {
                        int size = _mapResolution.Value;
                        if (cmd.Args != null && cmd.Args.TryGetValue("size", out var s) && int.TryParse(s, out var n)) size = n;
                        bool force = cmd.Args != null && cmd.Args.TryGetValue("force", out var f) && f == "true";
                        if (_worldReadyAt < 0f) cmd.Result = new CommandResult { Ok = false, Message = "world not loaded" };
                        else
                        {
                            _map.Begin(_worldSeed, size, _cacheDir, force);
                            cmd.Result = new CommandResult { Ok = true, Message = _map.Current.ToString() };
                        }
                    }
                    catch (Exception e) { cmd.Result = new CommandResult { Ok = false, Message = e.Message }; }
                    finally { cmd.Done.Set(); }
                    continue;
                }
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
                if (kind == "player.death")
                {
                    _deathEvents.Add(new KeyValuePair<long, string>(_eventSeq, json));
                    try
                    {
                        Directory.CreateDirectory(Path.GetDirectoryName(_deathJournalPath));
                        File.AppendAllText(_deathJournalPath, json + "\n", Encoding.UTF8);
                    }
                    catch (Exception e) { Logger.LogWarning("death journal write failed: " + e.Message); }
                }
                while (_events.Count > EventBuffer) _events.RemoveFirst();
            }
        }

        private void LoadDeathJournal()
        {
            try
            {
                if (!File.Exists(_deathJournalPath)) return;
                foreach (var line in File.ReadLines(_deathJournalPath))
                {
                    // Journal lines are written by this plugin, one complete event per line.
                    // Ignore an incomplete final line after a crash.
                    var marker = "\"seq\":";
                    int start = line.IndexOf(marker, StringComparison.Ordinal);
                    if (start < 0 || !line.EndsWith("}}", StringComparison.Ordinal) || line.IndexOf("\"kind\":\"player.death\"", StringComparison.Ordinal) < 0) continue;
                    start += marker.Length;
                    int end = line.IndexOf(',', start);
                    if (end < 0 || !long.TryParse(line.Substring(start, end - start), NumberStyles.None, CultureInfo.InvariantCulture, out var seq)) continue;
                    _deathEvents.Add(new KeyValuePair<long, string>(seq, line));
                    if (seq > _eventSeq) _eventSeq = seq;
                }
            }
            catch (Exception e) { Logger.LogWarning("death journal read failed: " + e.Message); }
        }

        // The manager only advances `since` after storing every death in that
        // response. Once it asks for a later sequence, older journal records
        // can be removed without losing replay after either process restarts.
        private void CompactDeathJournal(long acknowledged)
        {
            int count = 0;
            while (count < _deathEvents.Count && _deathEvents[count].Key <= acknowledged) count++;
            if (count == 0) return;
            var temp = _deathJournalPath + ".tmp";
            try
            {
                using (var writer = new StreamWriter(temp, false, new UTF8Encoding(false)))
                {
                    for (int i = count; i < _deathEvents.Count; i++) writer.WriteLine(_deathEvents[i].Value);
                }
                File.Replace(temp, _deathJournalPath, null);
                _deathEvents.RemoveRange(0, count);
            }
            catch (Exception e)
            {
                Logger.LogWarning("death journal compaction failed: " + e.Message);
                try { if (File.Exists(temp)) File.Delete(temp); } catch (Exception) { }
            }
        }

        internal void RecordDeath(Player player)
        {
            var snap = _latestSnapshot;
            if (player == null || snap == null || !snap.Ready || snap.WorldUID == 0) return;
            long playerID = player.GetPlayerID();
            if (playerID == 0) return;
            var pos = player.transform.position;
            bool visible = false;
            ZDOID objectID = ZDOID.None;
            try
            {
                var zdo = player.GetComponent<ZNetView>()?.GetZDO();
                if (zdo != null)
                {
                    objectID = zdo.m_uid;
                    foreach (var peer in snap.Players)
                    {
                        if (peer.CharacterId == objectID.ToString()) { visible = peer.Visible; break; }
                    }
                }
            }
            catch (Exception) { }
            string biome = "";
            try { biome = player.GetCurrentBiome().ToString(); } catch (Exception) { }
            EmitDeath(snap, objectID, playerID, player.GetPlayerName(), pos, visible, biome);
        }

        // Dedicated servers can receive a player's ZDO without instantiating
        // its Player component, so RPC_OnDeath alone cannot capture every death.
        // The owning client sets this replicated flag before sending the RPC.
        private void RecordDeathsFromZDOs(StateSnapshot snap)
        {
            var zdoMan = ZDOMan.instance;
            var znet = ZNet.instance;
            if (zdoMan == null || znet == null || snap.WorldUID == 0) return;
            foreach (var peer in znet.GetPeers())
            {
                if (peer == null || !peer.IsReady() || peer.m_characterID == ZDOID.None) continue;
                try
                {
                    var zdo = zdoMan.GetZDO(peer.m_characterID);
                    if (zdo == null || !zdo.GetBool(ZDOVars.s_dead) || _recordedDeaths.Contains(peer.m_characterID)) continue;
                    long playerID = zdo.GetLong(ZDOVars.s_playerID, 0L);
                    if (playerID == 0) continue;
                    var name = zdo.GetString(ZDOVars.s_playerName, peer.m_playerName ?? "");
                    EmitDeath(snap, peer.m_characterID, playerID, name, zdo.GetPosition(), peer.m_publicRefPos, "", zdo);
                }
                catch (Exception e) { Logger.LogWarning("death ZDO capture failed: " + e.Message); }
            }
        }

        private void EmitDeath(StateSnapshot snap, ZDOID objectID, long playerID, string playerName, Vector3 pos, bool visible, string biome, ZDO deathZdo = null)
        {
            if (objectID != ZDOID.None && _recordedDeaths.Contains(objectID)) return;
            if (deathZdo == null && objectID != ZDOID.None) deathZdo = ZDOMan.instance?.GetZDO(objectID);
            string enemy = (deathZdo?.GetString(SurvivalDeathKeys.Enemy, "") ?? "").Trim();
            if (enemy.Length > 120) enemy = enemy.Substring(0, 120);
            int enemyLevel = enemy.Length == 0 ? 0 : Math.Max(0, Math.Min(100, deathZdo.GetInt(SurvivalDeathKeys.EnemyLevel)));
            string situation = deathZdo?.GetString(SurvivalDeathKeys.Situation, "") ?? "";
            if (situation.Length > 40 || !Enum.TryParse(situation, out HitData.HitType hitType)
                || !Enum.IsDefined(typeof(HitData.HitType), hitType) || hitType == HitData.HitType.Undefined) situation = "";
            if (string.IsNullOrEmpty(biome))
            {
                try
                {
                    var gen = WorldGenerator.instance;
                    if (gen != null)
                    {
                        var resolved = gen.GetBiome(pos);
                        if (resolved != Heightmap.Biome.None) biome = resolved.ToString();
                    }
                }
                catch (Exception) { }
            }
            int day = snap.Day;
            try { day = EnvMan.instance.GetDay(ZNet.instance.GetTimeSeconds()); } catch (Exception) { }
            var w = new JsonWriter();
            w.BeginObject();
            w.Prop("run_id", _runId);
            w.Prop("world_uid", snap.WorldUID);
            w.Prop("world_name", snap.WorldName);
            w.Prop("character_id", playerID.ToString(CultureInfo.InvariantCulture));
            w.Prop("player_name", playerName);
            w.Prop("day", day);
            w.Prop("enemy", enemy);
            w.Prop("enemy_level", enemyLevel);
            w.Prop("situation", situation);
            w.Prop("biome", biome);
            w.Prop("x", (double)pos.x);
            w.Prop("z", (double)pos.z);
            w.Prop("visible", visible);
            w.EndObject();
            PushEvent("player.death", w.ToString());
            if (objectID != ZDOID.None) _recordedDeaths.Add(objectID);
        }

        private string EventsSince(long since)
        {
            var w = new System.Text.StringBuilder(1024);
            lock (_eventsLock)
            {
                if (since > 0 && since <= _eventSeq) CompactDeathJournal(since);
                var merged = new SortedDictionary<long, string>();
                // The journal is ordered by sequence. Find the first unread
                // death directly, and only build one response page.
                int low = 0, high = _deathEvents.Count;
                while (low < high)
                {
                    int mid = low + (high - low) / 2;
                    if (_deathEvents[mid].Key <= since) low = mid + 1;
                    else high = mid;
                }
                int bytes = 0;
                for (int i = low; i < _deathEvents.Count && bytes < 1024 * 1024; i++)
                {
                    var kv = _deathEvents[i];
                    merged[kv.Key] = kv.Value;
                    bytes += kv.Value.Length;
                }
                foreach (var kv in _events) if (kv.Key > since) merged[kv.Key] = kv.Value;
                long next = since;
                w.Append("{\"next\":");
                var entries = new StringBuilder(1024);
                bool first = true;
                foreach (var kv in merged)
                {
                    if (kv.Key <= since) continue;
                    if (!first) entries.Append(',');
                    first = false;
                    entries.Append(kv.Value);
                    next = kv.Key;
                    if (entries.Length >= 1024 * 1024) break;
                }
                if (first) next = _eventSeq;
                w.Append(next.ToString(CultureInfo.InvariantCulture)).Append(",\"events\":[").Append(entries).Append("]}");
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
