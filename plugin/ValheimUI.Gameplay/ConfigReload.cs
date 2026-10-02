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
    /// </summary>
    internal static class ConfigReload
    {
        private const double DebounceSeconds = 0.5;
        private const double ReloadGuardSeconds = 1.0;

        private static ConfigFile _config;
        private static ManualLogSource _log;
        private static FileSystemWatcher _watcher;
        private static readonly object Lock = new object();
        private static DateTime? _dirtyAt;
        private static DateTime _lastReload = DateTime.MinValue;

        /// <summary>Starts watching; safe to call once from Awake(). No-op if the cfg path is unusable.</summary>
        public static void Start(ConfigFile config, ManualLogSource log)
        {
            _config = config;
            _log = log;

            var path = config.ConfigFilePath;
            var dir = Path.GetDirectoryName(path);
            var file = Path.GetFileName(path);
            if (string.IsNullOrEmpty(dir) || string.IsNullOrEmpty(file))
            {
                log.LogWarning("config reload: could not determine the cfg directory/file from " + path + "; live reload is disabled.");
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
            lock (Lock) { _dirtyAt = DateTime.UtcNow; }
        }

        /// <summary>Called every Update(). Reloads once the file has been quiet for <see cref="DebounceSeconds"/>.</summary>
        public static void Poll()
        {
            DateTime? dirtyAt;
            lock (Lock) { dirtyAt = _dirtyAt; }
            if (dirtyAt == null) return;
            if ((DateTime.UtcNow - dirtyAt.Value).TotalSeconds < DebounceSeconds) return;

            lock (Lock) { _dirtyAt = null; }
            try
            {
                _config.Reload();
                _lastReload = DateTime.UtcNow;
                _log.LogInfo("config reloaded");
            }
            catch (Exception e)
            {
                _log.LogWarning("config reload failed: " + e.Message);
            }
        }
    }
}
