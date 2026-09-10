using System;
using System.Collections.Generic;
using System.Globalization;
using System.IO;
using HarmonyLib;
using UnityEngine;

namespace ValheimUI.Agent
{
    /// <summary>A location the game revealed to a player (a Vegvisir runestone).</summary>
    internal sealed class Discovery
    {
        public string Name;
        /// <summary>Minimap.PinType the game used (9 = Boss).</summary>
        public int Type;
        public Vector3 Pos;
        public DateTime At;
    }

    /// <summary>
    /// Locations the game itself put on players' maps. When a player reads a
    /// Vegvisir the client asks the server for the closest location of that
    /// kind and the server answers with a routed RPC "DiscoverLocationResponse"
    /// carrying the pin name, type and position; the patch below records that
    /// answer, so a boss found by anyone shows on the manager's map without a
    /// cartography table. Persisted per world in the agent's cache directory.
    /// </summary>
    internal sealed class Discoveries
    {
        private readonly object _lock = new object();
        private readonly List<Discovery> _items = new List<Discovery>();
        private string _path = "";
        private bool _dirty;
        private DateTime _lastSaved = DateTime.MinValue;
        private int _version;

        public int Version { get { lock (_lock) return _version; } }

        public void Load(int seed, string cacheDir)
        {
            lock (_lock)
            {
                _items.Clear();
                _dirty = false;
                _path = Path.Combine(cacheDir, "discovered-" + seed + ".tsv");
                try
                {
                    if (!File.Exists(_path)) return;
                    foreach (var line in File.ReadAllLines(_path))
                    {
                        var f = line.Split('\t');
                        if (f.Length < 6) continue;
                        int type;
                        float x, y, z;
                        DateTime at;
                        if (!int.TryParse(f[1], NumberStyles.Integer, CultureInfo.InvariantCulture, out type)) continue;
                        if (!float.TryParse(f[2], NumberStyles.Float, CultureInfo.InvariantCulture, out x)) continue;
                        if (!float.TryParse(f[3], NumberStyles.Float, CultureInfo.InvariantCulture, out y)) continue;
                        if (!float.TryParse(f[4], NumberStyles.Float, CultureInfo.InvariantCulture, out z)) continue;
                        if (!DateTime.TryParse(f[5], CultureInfo.InvariantCulture, DateTimeStyles.RoundtripKind, out at)) at = DateTime.UtcNow;
                        _items.Add(new Discovery { Name = f[0], Type = type, Pos = new Vector3(x, y, z), At = at });
                    }
                    _version++;
                }
                catch (Exception)
                {
                    _items.Clear();
                }
            }
        }

        /// <summary>Records a discovery unless the same spot is already known. Any thread.</summary>
        public bool Add(string name, int type, Vector3 pos)
        {
            lock (_lock)
            {
                foreach (var d in _items)
                {
                    if (d.Type == type && (d.Pos - pos).sqrMagnitude < 64f) return false;
                }
                _items.Add(new Discovery { Name = name ?? "", Type = type, Pos = pos, At = DateTime.UtcNow });
                _version++;
                _dirty = true;
                return true;
            }
        }

        public List<Discovery> Snapshot()
        {
            lock (_lock) return new List<Discovery>(_items);
        }

        /// <summary>Writes the list when it changed, at most every 10 s unless forced.</summary>
        public void MaybeSave(bool force)
        {
            List<Discovery> items;
            string path;
            lock (_lock)
            {
                if (!_dirty || _path.Length == 0) return;
                if (!force && (DateTime.UtcNow - _lastSaved).TotalSeconds < 10) return;
                items = new List<Discovery>(_items);
                path = _path;
                _dirty = false;
                _lastSaved = DateTime.UtcNow;
            }
            try
            {
                Directory.CreateDirectory(Path.GetDirectoryName(path));
                var lines = new List<string>(items.Count);
                foreach (var d in items)
                {
                    lines.Add(string.Join("\t", new[]
                    {
                        (d.Name ?? "").Replace('\t', ' ').Replace('\n', ' '),
                        d.Type.ToString(CultureInfo.InvariantCulture),
                        d.Pos.x.ToString("R", CultureInfo.InvariantCulture),
                        d.Pos.y.ToString("R", CultureInfo.InvariantCulture),
                        d.Pos.z.ToString("R", CultureInfo.InvariantCulture),
                        d.At.ToString("o", CultureInfo.InvariantCulture),
                    }));
                }
                var tmp = path + ".tmp";
                File.WriteAllLines(tmp, lines.ToArray());
                if (File.Exists(path)) File.Delete(path);
                File.Move(tmp, path);
            }
            catch (Exception)
            {
                lock (_lock) _dirty = true;
            }
        }
    }

    /// <summary>
    /// Sees the server's answer to a Vegvisir: ZRoutedRpc.InvokeRoutedRPC(long,
    /// string, params object[]) with method "DiscoverLocationResponse" and
    /// (pinName, pinType, position, ...) as parameters. Read-only; the call
    /// proceeds untouched. Arguments are taken positionally (__args) so the
    /// patch does not depend on the game's parameter names.
    /// </summary>
    [HarmonyPatch]
    internal static class DiscoverLocationPatch
    {
        private static System.Reflection.MethodBase TargetMethod()
        {
            return AccessTools.Method(typeof(ZRoutedRpc), "InvokeRoutedRPC", new[] { typeof(long), typeof(string), typeof(object[]) });
        }

        private static void Prefix(object[] __args)
        {
            try
            {
                if (__args == null || __args.Length < 3) return;
                var method = __args[1] as string;
                var ps = __args[2] as object[];
                if (method != "DiscoverLocationResponse" || ps == null || ps.Length < 3) return;
                if (!(ps[2] is Vector3)) return;
                var name = ps[0] as string ?? "";
                int type = ps[1] is int ? (int)ps[1] : 0;
                var store = AgentPlugin.Discoveries;
                if (store != null && store.Add(name, type, (Vector3)ps[2]))
                {
                    AgentPlugin.Log?.LogInfo("location discovered: " + name + " at " + ((Vector3)ps[2]).ToString());
                }
            }
            catch (Exception)
            {
            }
        }
    }
}
