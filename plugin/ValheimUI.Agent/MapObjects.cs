using System;
using System.Collections.Generic;
using UnityEngine;

namespace ValheimUI.Agent
{
    /// <summary>
    /// Points of interest read from the server's ZDO store: portals with their
    /// tags, ships, carts, tombstones, claimed beds, and the boss locations the
    /// game itself marks on the map. Refreshed one prefab per frame so a large
    /// world never stalls the server; the JSON is rebuilt when a pass completes.
    /// </summary>
    internal sealed class MapObjects
    {
        private sealed class Kind
        {
            public string Type;
            public string Prefab;
            public string Label;
            public string TextKey;
        }

        private static readonly Kind[] Kinds =
        {
            new Kind { Type = "portal", Prefab = "portal_wood", Label = "Portal", TextKey = "tag" },
            new Kind { Type = "portal", Prefab = "portal_stone", Label = "Stone portal", TextKey = "tag" },
            new Kind { Type = "ship", Prefab = "Raft", Label = "Raft" },
            new Kind { Type = "ship", Prefab = "Karve", Label = "Karve" },
            new Kind { Type = "ship", Prefab = "VikingShip", Label = "Longship" },
            new Kind { Type = "ship", Prefab = "VikingShip_Ashlands", Label = "Drakkar" },
            new Kind { Type = "cart", Prefab = "Cart", Label = "Cart" },
            new Kind { Type = "tombstone", Prefab = "Player_tombstone", Label = "Tombstone", TextKey = "ownerName" },
            new Kind { Type = "bed", Prefab = "bed", Label = "Bed", TextKey = "ownerName" },
            new Kind { Type = "bed", Prefab = "piece_bed02", Label = "Dragon bed", TextKey = "ownerName" },
        };

        private readonly TimeSpan _refreshEvery = TimeSpan.FromSeconds(30);
        private readonly List<ZDO> _buffer = new List<ZDO>(256);
        private readonly System.Text.StringBuilder _pending = new System.Text.StringBuilder(4096);
        private int _kindIndex = -1;
        private bool _firstEntry = true;
        private DateTime _lastCompleted = DateTime.MinValue;
        private volatile string _json = "{\"objects\":[],\"locations\":[],\"updated_at\":null}";

        public string Json => _json;

        /// <summary>Advances the scan by one prefab. Main thread.</summary>
        public void Step()
        {
            var zdoman = ZDOMan.instance;
            if (zdoman == null) return;
            if (_kindIndex < 0)
            {
                if (DateTime.UtcNow - _lastCompleted < _refreshEvery) return;
                _kindIndex = 0;
                _pending.Length = 0;
                _pending.Append("{\"objects\":[");
                _firstEntry = true;
            }
            if (_kindIndex < Kinds.Length)
            {
                var kind = Kinds[_kindIndex];
                _buffer.Clear();
                try
                {
                    zdoman.GetAllZDOsWithPrefab(kind.Prefab, _buffer);
                }
                catch (Exception)
                {
                    _buffer.Clear();
                }
                foreach (var zdo in _buffer)
                {
                    if (zdo == null) continue;
                    var pos = zdo.GetPosition();
                    string text = "";
                    if (kind.TextKey != null)
                    {
                        try { text = zdo.GetString(kind.TextKey, "") ?? ""; } catch (Exception) { text = ""; }
                    }
                    if (!_firstEntry) _pending.Append(',');
                    _firstEntry = false;
                    _pending.Append("{\"type\":").Append(JsonWriter.Quote(kind.Type))
                        .Append(",\"label\":").Append(JsonWriter.Quote(kind.Label))
                        .Append(",\"x\":").Append(F(pos.x))
                        .Append(",\"y\":").Append(F(pos.y))
                        .Append(",\"z\":").Append(F(pos.z))
                        .Append(",\"text\":").Append(JsonWriter.Quote(text))
                        .Append('}');
                }
                _kindIndex++;
                return;
            }

            // Last step of a pass: the game's own location icons (boss altars,
            // start temple, trader once found) and the timestamp.
            _pending.Append("],\"locations\":[");
            try
            {
                var icons = new Dictionary<Vector3, string>();
                var zs = ZoneSystem.instance;
                if (zs != null) zs.GetLocationIcons(icons);
                bool first = true;
                foreach (var kv in icons)
                {
                    if (!first) _pending.Append(',');
                    first = false;
                    _pending.Append("{\"name\":").Append(JsonWriter.Quote(kv.Value))
                        .Append(",\"x\":").Append(F(kv.Key.x))
                        .Append(",\"y\":").Append(F(kv.Key.y))
                        .Append(",\"z\":").Append(F(kv.Key.z))
                        .Append('}');
                }
            }
            catch (Exception)
            {
            }
            _pending.Append("],\"updated_at\":").Append(JsonWriter.Quote(DateTime.UtcNow.ToString("o"))).Append('}');
            _json = _pending.ToString();
            _lastCompleted = DateTime.UtcNow;
            _kindIndex = -1;
        }

        private static string F(float v)
        {
            return Math.Round(v, 1).ToString(System.Globalization.CultureInfo.InvariantCulture);
        }
    }
}
