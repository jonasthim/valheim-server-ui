using System;
using System.IO;
using BepInEx.Configuration;
using BepInEx.Logging;

namespace ValheimUI.Gameplay
{
    /// <summary>
    /// Watches the plugin's own cfg file and reloads it a short debounce
    /// after the manager finishes writing it. The manager writes atomically
    /// (tmp file + rename), so a write shows up here as Renamed, not just
    /// Changed. BepInEx is not thread-safe: the <see cref="FileSystemWatcher"/>
    /// callbacks only record that the file is dirty; the actual
    /// <see cref="ConfigFile.Reload"/> call happens from <see cref="Poll"/>,
    /// called from the plugin's own <c>Update()</c> on the main thread.
    ///
    /// <see cref="FileSystemWatcher"/> on Mono/Linux (the dedicated server's
    /// usual runtime) is known to miss events under some filesystems/editors,
    /// so <see cref="Poll"/> also stats the file itself at most every
    /// <see cref="StatPollSeconds"/> and treats a changed last-write-time or
    /// length exactly like a watcher event — a slower but reliable fallback.
    /// The watcher stays enabled too: it is faster when it works.
    /// </summary>
    internal static class ConfigReload
    {
        private const double DebounceSeconds = 0.5;
        private const double ReloadGuardSeconds = 1.0;
        private const double StatPollSeconds = 2.0;

        private static ConfigFile _config;
        private static ManualLogSource _log;
        private static FileSystemWatcher _watcher;
        private static string _path;
        private static readonly object Lock = new object();
        private static DateTime? _dirtyAt;
        private static DateTime _lastReload = DateTime.MinValue;
        private static DateTime _lastStatPoll = DateTime.MinValue;
        private static DateTime _lastSeenWriteTimeUtc;
        private static long _lastSeenLength = -1;
        private static string _dirtySource = "watcher";

        /// <summary>Starts watching; safe to call once from Awake(). Polling (see class remarks) still runs even if the watcher cannot be set up.</summary>
        public static void Start(ConfigFile config, ManualLogSource log)
        {
            _config = config;
            _log = log;

            var path = config.ConfigFilePath;
            _path = path;
            RecordStat();

            var dir = Path.GetDirectoryName(path);
            var file = Path.GetFileName(path);
            if (string.IsNullOrEmpty(dir) || string.IsNullOrEmpty(file))
            {
                log.LogWarning("config reload: could not determine the cfg directory/file from " + path + "; the watcher is disabled, live reload will rely on polling only.");
                return;
            }

            var watcher = new FileSystemWatcher(dir, file)
            {
                NotifyFilter = NotifyFilters.LastWrite | NotifyFilters.FileName | NotifyFilters.Size,
            };
            watcher.Changed += OnFileEvent;
            watcher.Created += OnFileEvent;
            watcher.Renamed += OnFileEvent;
            watcher.EnableRaisingEvents = true;
            _watcher = watcher;
        }

        /// <summary>Stops watching; called from OnDestroy().</summary>
        public static void Stop()
        {
            var watcher = _watcher;
            _watcher = null;
            if (watcher == null) return;
            watcher.EnableRaisingEvents = false;
            watcher.Changed -= OnFileEvent;
            watcher.Created -= OnFileEvent;
            watcher.Renamed -= OnFileEvent;
            watcher.Dispose();
        }

        private static void OnFileEvent(object sender, FileSystemEventArgs e)
        {
            // Guard against reload storms: a reload just happened, so this
            // event is most likely an echo of our own Config.Reload() (e.g.
            // BepInEx touching the file) rather than a fresh write by the
            // manager.
            if ((DateTime.UtcNow - _lastReload).TotalSeconds < ReloadGuardSeconds) return;
            lock (Lock) { _dirtyAt = DateTime.UtcNow; _dirtySource = "watcher"; }
        }

        /// <summary>
        /// Polling fallback for when the watcher's underlying OS notification
        /// never fires (observed on Mono/Linux). Runs at most every
        /// <see cref="StatPollSeconds"/>; marks the file dirty, exactly as a
        /// watcher event would, when its last-write-time or length differs
        /// from what was recorded at the previous successful reload or at
        /// <see cref="Start"/>.
        /// </summary>
        private static void CheckStatPoll()
        {
            if ((DateTime.UtcNow - _lastStatPoll).TotalSeconds < StatPollSeconds) return;
            _lastStatPoll = DateTime.UtcNow;

            // Same self-reload guard as the watcher: a reload just happened,
            // so a stat difference right now is most likely our own write,
            // not a fresh one from the manager.
            if ((DateTime.UtcNow - _lastReload).TotalSeconds < ReloadGuardSeconds) return;

            try
            {
                var info = new FileInfo(_path);
                if (!info.Exists) return; // vanished file is not an error, just skip this poll.
                var writeTimeUtc = File.GetLastWriteTimeUtc(_path);
                var length = info.Length;
                if (writeTimeUtc != _lastSeenWriteTimeUtc || length != _lastSeenLength)
                {
                    lock (Lock) { _dirtyAt = DateTime.UtcNow; _dirtySource = "polled"; }
                }
            }
            catch (Exception)
            {
                // Transient IO error (e.g. caught mid-write) or a vanished
                // file; not an error, just skip this poll and try again next time.
            }
        }

        /// <summary>Records the file's current last-write-time/length so the next <see cref="CheckStatPoll"/> has a baseline.</summary>
        private static void RecordStat()
        {
            try
            {
                var info = new FileInfo(_path);
                if (info.Exists)
                {
                    _lastSeenWriteTimeUtc = File.GetLastWriteTimeUtc(_path);
                    _lastSeenLength = info.Length;
                }
                else
                {
                    _lastSeenWriteTimeUtc = DateTime.MinValue;
                    _lastSeenLength = -1;
                }
            }
            catch (Exception)
            {
                // Vanished file or transient IO error; leave the previous
                // baseline in place rather than treating this as fatal.
            }
        }

        /// <summary>Called every Update(). Reloads once the file has been quiet for <see cref="DebounceSeconds"/>, whether the watcher or the polling fallback noticed the change.</summary>
        public static void Poll()
        {
            CheckStatPoll();

            DateTime? dirtyAt;
            string source;
            lock (Lock) { dirtyAt = _dirtyAt; source = _dirtySource; }
            if (dirtyAt == null) return;
            if ((DateTime.UtcNow - dirtyAt.Value).TotalSeconds < DebounceSeconds) return;

            lock (Lock) { _dirtyAt = null; }
            try
            {
                _config.Reload();
                _lastReload = DateTime.UtcNow;
                RecordStat();
                _log.LogInfo("config reloaded (" + source + ")");
            }
            catch (Exception e)
            {
                _log.LogWarning("config reload failed: " + e.Message);
            }
        }
    }
}
