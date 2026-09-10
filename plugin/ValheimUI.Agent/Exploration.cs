using System;
using System.Collections.Generic;
using System.IO;
using System.IO.Compression;
using System.Threading;
using UnityEngine;

namespace ValheimUI.Agent
{
    /// <summary>
    /// Fog of war for the manager's map. The game keeps exploration on each
    /// client, never on the server, so the agent reconstructs it: every player
    /// position the server sees reveals the game's 100 m radius around it, and
    /// the shared map stored in cartography tables (players who "write" their
    /// map) is imported, which brings in exploration made before the agent
    /// existed. The union is persisted per world so it survives restarts.
    /// </summary>
    internal sealed class Exploration
    {
        public const int Size = 1024;
        /// <summary>Minimap.m_exploreRadius in the game.</summary>
        public const float ExploreRadius = 100f;
        private const string Magic = "VUIEXPL1";

        private readonly object _lock = new object();
        private readonly byte[] _cells = new byte[Size * Size];
        private int _count;
        private int _version;
        private int _seed;
        private string _path = "";
        private DateTime _lastSaved = DateTime.MinValue;
        private int _savedVersion;
        private byte[] _png;
        private int _pngVersion = -1;
        private bool _encoding;
        private DateTime _updatedAt = DateTime.MinValue;
        private readonly Dictionary<ZDOID, int> _importedTables = new Dictionary<ZDOID, int>();

        public int Version { get { lock (_lock) return _version; } }
        public bool Loaded { get; private set; }

        public void Load(int seed, string cacheDir)
        {
            lock (_lock)
            {
                _seed = seed;
                _path = Path.Combine(cacheDir, "explored-" + seed + "-" + Size + ".bin");
                Array.Clear(_cells, 0, _cells.Length);
                _count = 0;
                _importedTables.Clear();
                try
                {
                    if (File.Exists(_path))
                    {
                        using (var br = new BinaryReader(File.OpenRead(_path)))
                        {
                            var magic = new string(br.ReadChars(Magic.Length));
                            int size = br.ReadInt32();
                            if (magic == Magic && size == Size)
                            {
                                var data = br.ReadBytes(Size * Size);
                                if (data.Length == Size * Size)
                                {
                                    Buffer.BlockCopy(data, 0, _cells, 0, data.Length);
                                    foreach (var b in _cells) if (b != 0) _count++;
                                }
                            }
                        }
                    }
                }
                catch (Exception)
                {
                    Array.Clear(_cells, 0, _cells.Length);
                    _count = 0;
                }
                _version++;
                _savedVersion = _version;
                _updatedAt = DateTime.UtcNow;
                Loaded = true;
            }
        }

        /// <summary>Reveals a disc around a world position. Main thread.</summary>
        public void Explore(Vector3 pos, float radiusMeters)
        {
            float step = 2f * MapRenderer.WorldRadius / Size;
            int cx = Mathf.FloorToInt((pos.x + MapRenderer.WorldRadius) / step);
            int cz = Mathf.FloorToInt((MapRenderer.WorldRadius - pos.z) / step);
            int r = Mathf.CeilToInt(radiusMeters / step);
            int r2 = r * r;
            int added = 0;
            lock (_lock)
            {
                for (int dz = -r; dz <= r; dz++)
                {
                    int z = cz + dz;
                    if (z < 0 || z >= Size) continue;
                    for (int dx = -r; dx <= r; dx++)
                    {
                        if (dx * dx + dz * dz > r2) continue;
                        int x = cx + dx;
                        if (x < 0 || x >= Size) continue;
                        int i = z * Size + x;
                        if (_cells[i] == 0) { _cells[i] = 1; added++; }
                    }
                }
                if (added > 0)
                {
                    _count += added;
                    _version++;
                    _updatedAt = DateTime.UtcNow;
                }
            }
        }

        public bool IsExplored(Vector3 pos)
        {
            float step = 2f * MapRenderer.WorldRadius / Size;
            int x = Mathf.FloorToInt((pos.x + MapRenderer.WorldRadius) / step);
            int z = Mathf.FloorToInt((MapRenderer.WorldRadius - pos.z) / step);
            if (x < 0 || x >= Size || z < 0 || z >= Size) return false;
            lock (_lock) return _cells[z * Size + x] != 0;
        }

        /// <summary>Persists at most once a minute when something changed. Any thread.</summary>
        public void MaybeSave(bool force)
        {
            byte[] copy;
            int version;
            lock (_lock)
            {
                if (!Loaded || string.IsNullOrEmpty(_path)) return;
                if (_version == _savedVersion) return;
                if (!force && (DateTime.UtcNow - _lastSaved).TotalSeconds < 60) return;
                copy = (byte[])_cells.Clone();
                version = _version;
                _lastSaved = DateTime.UtcNow;
            }
            try
            {
                Directory.CreateDirectory(Path.GetDirectoryName(_path));
                using (var bw = new BinaryWriter(File.Create(_path + ".tmp")))
                {
                    bw.Write(Magic.ToCharArray());
                    bw.Write(Size);
                    bw.Write(copy);
                }
                if (File.Exists(_path)) File.Delete(_path);
                File.Move(_path + ".tmp", _path);
                lock (_lock) _savedVersion = version;
            }
            catch (Exception)
            {
            }
        }

        /// <summary>
        /// Imports the shared map of a cartography table (its "data" ZDO field):
        /// a gzip'd ZPackage with version, texture size and one bool per map
        /// pixel (12 m each, centred on the world). Only the explored bits are
        /// read; pins and later fields are ignored. Best effort per table.
        /// </summary>
        public void ImportSharedMap(ZDOID table, byte[] data)
        {
            if (data == null || data.Length < 8) return;
            int key = data.Length ^ (data[data.Length - 1] << 8) ^ (data[data.Length / 2] << 16);
            lock (_lock)
            {
                int seen;
                if (_importedTables.TryGetValue(table, out seen) && seen == key) return;
                _importedTables[table] = key;
            }
            byte[] raw;
            try
            {
                using (var ms = new MemoryStream(data))
                using (var gz = new GZipStream(ms, CompressionMode.Decompress))
                using (var outMs = new MemoryStream())
                {
                    gz.CopyTo(outMs);
                    raw = outMs.ToArray();
                }
            }
            catch (Exception)
            {
                raw = data; // older, uncompressed layout
            }
            try
            {
                var pkg = new ZPackage(raw);
                int version = pkg.ReadInt();
                int textureSize = pkg.ReadInt();
                if (version < 1 || textureSize < 64 || textureSize > 8192) return;
                float pixelSize = 12f * (2048f / textureSize); // Minimap.m_pixelSize at its default texture size
                float half = textureSize / 2f;
                float step = 2f * MapRenderer.WorldRadius / Size;
                int added = 0;
                lock (_lock)
                {
                    for (int i = 0; i < textureSize * textureSize; i++)
                    {
                        if (!pkg.ReadBool()) continue;
                        float wx = (i % textureSize - half) * pixelSize;
                        float wz = (i / textureSize - half) * pixelSize;
                        int x = Mathf.FloorToInt((wx + MapRenderer.WorldRadius) / step);
                        int z = Mathf.FloorToInt((MapRenderer.WorldRadius - wz) / step);
                        if (x < 0 || x >= Size || z < 0 || z >= Size) continue;
                        int idx = z * Size + x;
                        if (_cells[idx] == 0) { _cells[idx] = 1; added++; }
                    }
                    if (added > 0)
                    {
                        _count += added;
                        _version++;
                        _updatedAt = DateTime.UtcNow;
                    }
                }
            }
            catch (Exception)
            {
            }
        }

        /// <summary>
        /// The fog mask as a grey+alpha PNG: 255 where unexplored (fog drawn),
        /// 0 where explored. Encoded on a worker thread per version; the
        /// previous version is served meanwhile.
        /// </summary>
        public byte[] MaskPng()
        {
            byte[] current;
            bool kick = false;
            lock (_lock)
            {
                current = _png;
                if (_pngVersion != _version && !_encoding)
                {
                    _encoding = true;
                    kick = true;
                }
            }
            if (kick)
            {
                byte[] snapshot;
                int version;
                lock (_lock)
                {
                    snapshot = (byte[])_cells.Clone();
                    version = _version;
                }
                ThreadPool.QueueUserWorkItem(_ =>
                {
                    try
                    {
                        // Luminance and alpha both carry the mask (255 = fog), so
                        // browsers masking by alpha or by luminance agree.
                        var ga = new byte[snapshot.Length * 2];
                        for (int i = 0; i < snapshot.Length; i++)
                        {
                            byte v = snapshot[i] != 0 ? (byte)0 : (byte)255;
                            ga[i * 2] = v;
                            ga[i * 2 + 1] = v;
                        }
                        var png = Png.EncodeGrayAlpha(Size, Size, ga);
                        lock (_lock)
                        {
                            _png = png;
                            _pngVersion = version;
                            _encoding = false;
                        }
                    }
                    catch (Exception)
                    {
                        lock (_lock) _encoding = false;
                    }
                });
            }
            return current;
        }

        public string InfoJson()
        {
            var w = new JsonWriter();
            w.BeginObject();
            lock (_lock)
            {
                w.Prop("version", _version);
                w.Prop("size", Size);
                w.Prop("explored_cells", _count);
                w.Prop("total_cells", Size * Size);
                w.Prop("percent", Math.Round(100.0 * _count / (Size * Size), 2));
                w.Prop("updated_at", _updatedAt == DateTime.MinValue ? null : _updatedAt.ToString("o"));
                w.Prop("mask_version", _pngVersion);
            }
            w.EndObject();
            return w.ToString();
        }
    }
}
