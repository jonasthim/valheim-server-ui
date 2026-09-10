using System;
using System.Diagnostics;
using System.IO;
using System.Threading;
using UnityEngine;

namespace ValheimUI.Agent
{
    /// <summary>
    /// Renders the world map from the seed: samples WorldGenerator's biome and
    /// height on a grid covering the whole world and colours it like the
    /// in-game map, without fog. Sampling runs on the main thread in small
    /// per-frame slices (WorldGenerator is only guaranteed there); PNG
    /// encoding runs on a worker thread. The result is cached on disk per
    /// seed and size so a restart serves it instantly.
    /// </summary>
    internal sealed class MapRenderer
    {
        public enum State { Idle, Rendering, Encoding, Ready, Failed }

        /// <summary>Half-width of the rendered square in metres. Valheim's playable
        /// world is a 10 000 m radius disc; the edge of the world sits at ~10 500.</summary>
        public const float WorldRadius = 10500f;
        private const float PlayableRadius = 10000f;
        private const float SeaLevel = 30f;

        private readonly object _lock = new object();
        private volatile State _state = State.Idle;
        private byte[] _png;
        private byte[] _rgb;
        private int _size;
        private int _seed;
        private int _nextRow;
        private string _cachePath;
        private string _error = "";

        public State Current => _state;
        public int Size => _size;
        public int Seed => _seed;
        public string Error => _error;
        public float Progress => _size == 0 ? 0f : Math.Min(1f, (float)_nextRow / _size);

        public byte[] Png()
        {
            lock (_lock) return _png;
        }

        /// <summary>Starts a render (or loads the cached image). Main thread.</summary>
        public void Begin(int seed, int size, string cacheDir, bool force)
        {
            size = Mathf.Clamp(size, 256, 4096);
            lock (_lock)
            {
                if (_state == State.Rendering || _state == State.Encoding) return;
                _seed = seed;
                _size = size;
                _cachePath = Path.Combine(cacheDir, "map-" + seed.ToString() + "-" + size + ".png");
                _error = "";
                _png = null;
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
                _rgb = new byte[size * size * 3];
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
            var sw = Stopwatch.StartNew();
            while (_nextRow < _size && sw.Elapsed.TotalMilliseconds < budgetMs)
            {
                RenderRow(gen, _nextRow);
                _nextRow++;
            }
            if (_nextRow >= _size)
            {
                _state = State.Encoding;
                var rgb = _rgb;
                var size = _size;
                var path = _cachePath;
                _rgb = null;
                ThreadPool.QueueUserWorkItem(_ => Encode(size, rgb, path));
            }
        }

        private void Encode(int size, byte[] rgb, string path)
        {
            try
            {
                var png = global::ValheimUI.Agent.Png.EncodeRgb(size, size, rgb);
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
            float prevH = SeaLevel;
            int row = py * size * 3;
            for (int px = 0; px < size; px++)
            {
                float wx = -WorldRadius + (px + 0.5f) * step;
                float dist = Mathf.Sqrt(wx * wx + wz * wz);
                byte r, g, b;
                if (dist > WorldRadius)
                {
                    r = 18; g = 22; b = 30;
                }
                else
                {
                    var biome = gen.GetBiome(wx, wz);
                    float h = gen.GetHeight(wx, wz);
                    Colour(biome, h, h - prevH, out r, out g, out b);
                    if (dist > PlayableRadius)
                    {
                        float f = 1f - Mathf.Clamp01((dist - PlayableRadius) / (WorldRadius - PlayableRadius)) * 0.7f;
                        r = (byte)(r * f); g = (byte)(g * f); b = (byte)(b * f);
                    }
                    prevH = h;
                }
                int i = row + px * 3;
                _rgb[i] = r; _rgb[i + 1] = g; _rgb[i + 2] = b;
            }
        }

        private static void Colour(Heightmap.Biome biome, float h, float slope, out byte r, out byte g, out byte b)
        {
            if (h < SeaLevel)
            {
                float depth = Mathf.Clamp01((SeaLevel - h) / 60f);
                Lerp(70, 130, 190, 24, 52, 105, depth, out r, out g, out b);
                return;
            }
            int br, bg, bb;
            switch (biome)
            {
                case Heightmap.Biome.Meadows: br = 92; bg = 146; bb = 62; break;
                case Heightmap.Biome.BlackForest: br = 44; bg = 86; bb = 44; break;
                case Heightmap.Biome.Swamp: br = 96; bg = 82; bb = 56; break;
                case Heightmap.Biome.Mountain:
                    if (h > 120) { br = 220; bg = 224; bb = 232; } else { br = 150; bg = 152; bb = 160; }
                    break;
                case Heightmap.Biome.Plains: br = 194; bg = 172; bb = 92; break;
                case Heightmap.Biome.Mistlands: br = 112; bg = 100; bb = 122; break;
                case Heightmap.Biome.AshLands: br = 142; bg = 62; bb = 42; break;
                case Heightmap.Biome.DeepNorth: br = 208; bg = 220; bb = 236; break;
                case Heightmap.Biome.Ocean: br = 60; bg = 110; bb = 170; break;
                default: br = 84; bg = 84; bb = 84; break;
            }
            // Shore band, then brighten with altitude and a light east-west hillshade.
            if (h < SeaLevel + 1.5f) { br = 200; bg = 190; bb = 150; }
            float f = 0.78f + 0.45f * Mathf.Clamp01((h - SeaLevel) / 220f) + Mathf.Clamp(slope * 0.03f, -0.22f, 0.22f);
            r = (byte)Mathf.Clamp(br * f, 0, 255);
            g = (byte)Mathf.Clamp(bg * f, 0, 255);
            b = (byte)Mathf.Clamp(bb * f, 0, 255);
        }

        private static void Lerp(int r0, int g0, int b0, int r1, int g1, int b1, float t, out byte r, out byte g, out byte b)
        {
            r = (byte)(r0 + (r1 - r0) * t);
            g = (byte)(g0 + (g1 - g0) * t);
            b = (byte)(b0 + (b1 - b0) * t);
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
            w.Prop("error", _error);
            w.EndObject();
            return w.ToString();
        }
    }
}
