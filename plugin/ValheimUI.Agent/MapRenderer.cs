using System;
using System.Diagnostics;
using System.IO;
using System.Reflection;
using System.Threading;
using UnityEngine;

namespace ValheimUI.Agent
{
    /// <summary>
    /// Samples the world once per world into raw map layers: for every cell of
    /// a grid covering the whole world square, the biome, the height with the
    /// game's terrain mask (GetBiomeHeight) and the forest factor. The manager
    /// draws the map from these layers in the in-game style at any zoom; the
    /// plugin itself does no styling. Sampling runs on the main thread in
    /// small per-frame slices (WorldGenerator is only guaranteed there); PNG
    /// encoding runs on a worker thread. The result is cached on disk per seed
    /// and size so a restart serves it instantly.
    ///
    /// Layer image: 8-bit RGBA PNG. R = terrain mask (high nibble, 0-15) and
    /// biome code (low nibble); G:B = height as (h + 200) * 32, big endian;
    /// A = forest factor * 100. Mirrored by the manager's mapstyle package.
    /// </summary>
    internal sealed class MapRenderer
    {
        public enum State { Idle, Rendering, Encoding, Ready, Failed }

        /// <summary>Half-width of the rendered square in metres. Valheim's playable
        /// world is a 10 000 m radius disc; the edge of the world sits at ~10 500.</summary>
        public const float WorldRadius = 10500f;
        private const float PlayableRadius = 10000f;
        private const float SeaLevel = 30f;
        public const int LayersVersion = 1;

        private const float HeightOffset = 200f;
        private const float HeightScale = 32f;
        private const float ForestScale = 100f;
        private const byte BiomeOffWorld = 15;
        /// <summary>Forest factor written when GetForestFactor is unavailable: no trees.</summary>
        private const float NoForest = 2.55f;

        private delegate float BiomeHeightFn(Heightmap.Biome biome, float wx, float wy, out Color mask);
        private delegate float BiomeHeightPreFn(Heightmap.Biome biome, float wx, float wy, out Color mask, bool preGeneration);

        private readonly object _lock = new object();
        private volatile State _state = State.Idle;
        private byte[] _png;
        private byte[] _rgba;
        private int _size;
        private int _seed;
        private int _nextRow;
        private string _cachePath;
        private string _error = "";

        private WorldGenerator _boundGen;
        private BiomeHeightFn _biomeHeight;
        private Func<Vector3, float> _forest;

        public State Current => _state;
        public int Size => _size;
        public int Seed => _seed;
        public string Error => _error;
        public float Progress => _size == 0 ? 0f : Math.Min(1f, (float)_nextRow / _size);

        public byte[] Png()
        {
            lock (_lock) return _png;
        }

        /// <summary>Starts a render (or loads the cached layers). Main thread.</summary>
        public void Begin(int seed, int size, string cacheDir, bool force)
        {
            size = Mathf.Clamp(size, 256, 4096);
            lock (_lock)
            {
                if (_state == State.Rendering || _state == State.Encoding) return;
                _seed = seed;
                _size = size;
                _cachePath = Path.Combine(cacheDir, "layers-" + seed.ToString() + "-" + size + ".png");
                _error = "";
                _png = null;
                // The styled image older agents cached is no longer produced.
                try
                {
                    var legacy = Path.Combine(cacheDir, "map-" + seed.ToString() + "-" + size + ".png");
                    if (File.Exists(legacy)) File.Delete(legacy);
                }
                catch (Exception)
                {
                }
                if (!force && File.Exists(_cachePath))
                {
                    try
                    {
                        _png = File.ReadAllBytes(_cachePath);
                        _state = State.Ready;
                        return;
                    }
                    catch (Exception e)
                    {
                        _error = "cache read failed: " + e.Message;
                    }
                }
                _rgba = new byte[size * size * 4];
                _nextRow = 0;
                _state = State.Rendering;
            }
        }

        /// <summary>Renders rows until budgetMs is spent. Main thread.</summary>
        public void Step(float budgetMs)
        {
            if (_state != State.Rendering) return;
            var gen = WorldGenerator.instance;
            if (gen == null) return;
            if (gen != _boundGen) Bind(gen);
            var sw = Stopwatch.StartNew();
            while (_nextRow < _size && sw.Elapsed.TotalMilliseconds < budgetMs)
            {
                try
                {
                    RenderRow(gen, _nextRow);
                }
                catch (Exception e)
                {
                    // A game update changed one of the optional calls under us:
                    // drop to the next fallback and redo the row.
                    if (_biomeHeight != null)
                    {
                        _biomeHeight = null;
                        AgentPlugin.Log?.LogWarning("map layers: GetBiomeHeight failed, using GetHeight without the terrain mask: " + e.Message);
                        continue;
                    }
                    if (_forest != null)
                    {
                        _forest = null;
                        AgentPlugin.Log?.LogWarning("map layers: GetForestFactor failed, forest layer left empty: " + e.Message);
                        continue;
                    }
                    _error = "sampling failed: " + e.Message;
                    _state = State.Failed;
                    _rgba = null;
                    return;
                }
                _nextRow++;
            }
            if (_nextRow >= _size)
            {
                _state = State.Encoding;
                var rgba = _rgba;
                var size = _size;
                var path = _cachePath;
                _rgba = null;
                ThreadPool.QueueUserWorkItem(_ => Encode(size, rgba, path));
            }
        }

        /// <summary>
        /// Binds the optional WorldGenerator members once per generator
        /// instance. GetBiomeHeight (with the terrain mask) and the static
        /// GetForestFactor are resolved by reflection into delegates, so the
        /// plugin does not pin the exact signature at compile time yet pays no
        /// reflection cost per pixel. Missing members degrade to GetHeight
        /// (no mask) and "no forest".
        /// </summary>
        private void Bind(WorldGenerator gen)
        {
            _boundGen = gen;
            _biomeHeight = null;
            _forest = null;
            try
            {
                var t = typeof(WorldGenerator);
                var byRefColor = typeof(Color).MakeByRefType();
                var m5 = t.GetMethod("GetBiomeHeight", BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.Instance, null,
                    new[] { typeof(Heightmap.Biome), typeof(float), typeof(float), byRefColor, typeof(bool) }, null);
                if (m5 != null)
                {
                    var d = (BiomeHeightPreFn)Delegate.CreateDelegate(typeof(BiomeHeightPreFn), gen, m5);
                    _biomeHeight = (Heightmap.Biome b, float x, float y, out Color mk) => d(b, x, y, out mk, false);
                }
                else
                {
                    var m4 = t.GetMethod("GetBiomeHeight", BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.Instance, null,
                        new[] { typeof(Heightmap.Biome), typeof(float), typeof(float), byRefColor }, null);
                    if (m4 != null) _biomeHeight = (BiomeHeightFn)Delegate.CreateDelegate(typeof(BiomeHeightFn), gen, m4);
                    else _biomeHeight = BindBiomeHeightByShape(gen, t);
                }
                var fm = t.GetMethod("GetForestFactor", BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.Static, null,
                    new[] { typeof(Vector3) }, null);
                if (fm != null) _forest = (Func<Vector3, float>)Delegate.CreateDelegate(typeof(Func<Vector3, float>), fm);
            }
            catch (Exception e)
            {
                AgentPlugin.Log?.LogWarning("map layers: binding WorldGenerator members failed: " + e.Message);
            }
            if (_biomeHeight == null) AgentPlugin.Log?.LogWarning("map layers: GetBiomeHeight not found; the terrain mask (lava, mist) will be empty");
            if (_forest == null) AgentPlugin.Log?.LogWarning("map layers: GetForestFactor not found; the forest layer will be empty");
        }

        /// <summary>
        /// Last resort when neither known GetBiomeHeight signature exists:
        /// any instance method of that name taking (biome, x, y, out mask,
        /// ...) is called through reflection, trailing parameters at their
        /// defaults and the mask read back whether it is a Color or a
        /// Color32. Slower per sample, but the render is budgeted per frame
        /// anyway, and the bound signature is logged for the next update.
        /// </summary>
        private static BiomeHeightFn BindBiomeHeightByShape(WorldGenerator gen, Type t)
        {
            MethodInfo found = null;
            var seen = new System.Text.StringBuilder();
            foreach (var m in t.GetMethods(BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.Instance))
            {
                if (m.Name != "GetBiomeHeight") continue;
                var ps = m.GetParameters();
                seen.Append(' ').Append(Describe(m));
                if (ps.Length < 4 || !ps[0].ParameterType.IsEnum || ps[1].ParameterType != typeof(float) ||
                    ps[2].ParameterType != typeof(float) || !ps[3].ParameterType.IsByRef) continue;
                if (found == null || ps.Length < found.GetParameters().Length) found = m;
            }
            if (found == null)
            {
                AgentPlugin.Log?.LogWarning("map layers: no GetBiomeHeight with (biome, x, y, out mask, ...); candidates:" +
                    (seen.Length == 0 ? " none" : seen.ToString()));
                return null;
            }
            var pars = found.GetParameters();
            int n = pars.Length;
            var mi = found;
            AgentPlugin.Log?.LogInfo("map layers: GetBiomeHeight bound by reflection as " + Describe(mi));
            return (Heightmap.Biome b, float x, float y, out Color mk) =>
            {
                var args = new object[n];
                args[0] = Enum.ToObject(pars[0].ParameterType, (int)b);
                args[1] = x;
                args[2] = y;
                args[3] = null;
                for (int i = 4; i < n; i++)
                {
                    var pt = pars[i].ParameterType;
                    args[i] = pars[i].HasDefaultValue ? pars[i].DefaultValue : (pt.IsValueType ? Activator.CreateInstance(pt) : null);
                }
                var r = mi.Invoke(gen, args);
                mk = ToColor(args[3]);
                return Convert.ToSingle(r);
            };
        }

        private static Color ToColor(object v)
        {
            if (v is Color c) return c;
            if (v is Color32 c32) return c32;
            if (v is Vector4 v4) return new Color(v4.x, v4.y, v4.z, v4.w);
            return default(Color);
        }

        private static string Describe(MethodInfo m)
        {
            var sb = new System.Text.StringBuilder(m.Name).Append('(');
            var ps = m.GetParameters();
            for (int i = 0; i < ps.Length; i++)
            {
                if (i > 0) sb.Append(", ");
                sb.Append(ps[i].ParameterType.Name);
            }
            return sb.Append(')').ToString();
        }

        private void Encode(int size, byte[] rgba, string path)
        {
            try
            {
                var png = global::ValheimUI.Agent.Png.EncodeRgba(size, size, rgba);
                try
                {
                    Directory.CreateDirectory(Path.GetDirectoryName(path));
                    File.WriteAllBytes(path + ".tmp", png);
                    if (File.Exists(path)) File.Delete(path);
                    File.Move(path + ".tmp", path);
                }
                catch (Exception e)
                {
                    _error = "cache write failed: " + e.Message;
                }
                lock (_lock)
                {
                    _png = png;
                    _state = State.Ready;
                }
            }
            catch (Exception e)
            {
                _error = "encode failed: " + e.Message;
                _state = State.Failed;
            }
        }

        private void RenderRow(WorldGenerator gen, int py)
        {
            int size = _size;
            float step = 2f * WorldRadius / size;
            float wz = WorldRadius - (py + 0.5f) * step; // north (+z) at the top
            int row = py * size * 4;
            for (int px = 0; px < size; px++)
            {
                float wx = -WorldRadius + (px + 0.5f) * step;
                float dist = Mathf.Sqrt(wx * wx + wz * wz);
                byte code;
                float h, forest, mask;
                if (dist > WorldRadius)
                {
                    code = BiomeOffWorld;
                    h = SeaLevel - 50f;
                    forest = 0f;
                    mask = 0f;
                }
                else
                {
                    var biome = gen.GetBiome(wx, wz);
                    code = BiomeCode(biome);
                    if (_biomeHeight != null)
                    {
                        Color mk;
                        h = _biomeHeight(biome, wx, wz, out mk);
                        mask = mk.a;
                    }
                    else
                    {
                        h = gen.GetHeight(wx, wz);
                        mask = 0f;
                    }
                    forest = _forest != null ? _forest(new Vector3(wx, 0f, wz)) : NoForest;
                }
                int m = Mathf.Clamp(Mathf.RoundToInt(mask * 15f), 0, 15);
                int h16 = Mathf.Clamp(Mathf.RoundToInt((h + HeightOffset) * HeightScale), 0, 65535);
                int f = Mathf.Clamp(Mathf.RoundToInt(forest * ForestScale), 0, 255);
                int i = row + px * 4;
                _rgba[i] = (byte)((m << 4) | code);
                _rgba[i + 1] = (byte)(h16 >> 8);
                _rgba[i + 2] = (byte)(h16 & 0xFF);
                _rgba[i + 3] = (byte)f;
            }
        }

        private static byte BiomeCode(Heightmap.Biome biome)
        {
            switch (biome)
            {
                case Heightmap.Biome.Meadows: return 1;
                case Heightmap.Biome.BlackForest: return 2;
                case Heightmap.Biome.Swamp: return 3;
                case Heightmap.Biome.Mountain: return 4;
                case Heightmap.Biome.Plains: return 5;
                case Heightmap.Biome.Mistlands: return 6;
                case Heightmap.Biome.AshLands: return 7;
                case Heightmap.Biome.DeepNorth: return 8;
                case Heightmap.Biome.Ocean: return 9;
                default: return 0;
            }
        }

        public string InfoJson()
        {
            var w = new JsonWriter();
            w.BeginObject();
            w.Prop("state", _state.ToString().ToLowerInvariant());
            w.Prop("progress", (double)Progress);
            w.Prop("seed", _seed);
            w.Prop("size", _size);
            w.Prop("world_radius", (double)WorldRadius);
            w.Prop("playable_radius", (double)PlayableRadius);
            w.Prop("sea_level", (double)SeaLevel);
            w.Prop("layers", true);
            w.Prop("layers_version", LayersVersion);
            w.Prop("error", _error);
            w.EndObject();
            return w.ToString();
        }
    }
}
