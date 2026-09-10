using System;
using System.Collections.Generic;
using System.Reflection;
using UnityEngine;

namespace ValheimUI.Agent
{
    /// <summary>
    /// Points of interest read from the server's ZDO store: portals with their
    /// tags, ships, carts, tombstones, claimed beds, and the boss locations the
    /// game itself marks on the map. One pass over the ZDO table every 30 s,
    /// bucketing by prefab hash, so the cost is one dictionary walk regardless
    /// of how many kinds are tracked.
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

        private const int MaxObjects = 5000;
        private readonly TimeSpan _refreshEvery = TimeSpan.FromSeconds(30);
        private readonly Dictionary<int, Kind> _kindByHash = new Dictionary<int, Kind>();
        private DateTime _lastCompleted = DateTime.MinValue;
        private volatile string _json = "{\"objects\":[],\"locations\":[],\"updated_at\":null}";
        private bool _lookupResolved;
        private FieldInfo _objectsField;
        private MethodInfo _objectsMethod;

        public string Json => _json;

        public MapObjects()
        {
            foreach (var k in Kinds) _kindByHash[k.Prefab.GetStableHashCode()] = k;
        }

        /// <summary>Refreshes the object list when due. Main thread.</summary>
        public void Step()
        {
            var zdoman = ZDOMan.instance;
            if (zdoman == null) return;
            if (DateTime.UtcNow - _lastCompleted < _refreshEvery) return;
            _lastCompleted = DateTime.UtcNow;

            var sb = new System.Text.StringBuilder(8192);
            sb.Append("{\"objects\":[");
            int count = 0;
            try
            {
                var table = ObjectsById(zdoman);
                if (table != null)
                {
                    foreach (var kv in table)
                    {
                        var zdo = kv.Value;
                        if (zdo == null) continue;
                        Kind kind;
                        if (!_kindByHash.TryGetValue(zdo.GetPrefab(), out kind)) continue;
                        var pos = zdo.GetPosition();
                        string text = "";
                        if (kind.TextKey != null)
                        {
                            try { text = zdo.GetString(kind.TextKey, "") ?? ""; } catch (Exception) { text = ""; }
                        }
                        if (count > 0) sb.Append(',');
                        sb.Append("{\"type\":").Append(JsonWriter.Quote(kind.Type))
                            .Append(",\"label\":").Append(JsonWriter.Quote(kind.Label))
                            .Append(",\"x\":").Append(F(pos.x))
                            .Append(",\"y\":").Append(F(pos.y))
                            .Append(",\"z\":").Append(F(pos.z))
                            .Append(",\"text\":").Append(JsonWriter.Quote(text))
                            .Append('}');
                        if (++count >= MaxObjects) break;
                    }
                }
            }
            catch (InvalidOperationException)
            {
                // The table changed under us; try again next time.
                _lastCompleted = DateTime.UtcNow - _refreshEvery + TimeSpan.FromSeconds(2);
                return;
            }
            catch (Exception)
            {
            }

            sb.Append("],\"locations\":[");
            try
            {
                var icons = new Dictionary<Vector3, string>();
                var zs = ZoneSystem.instance;
                if (zs != null) zs.GetLocationIcons(icons);
                bool first = true;
                foreach (var kv in icons)
                {
                    if (!first) sb.Append(',');
                    first = false;
                    sb.Append("{\"name\":").Append(JsonWriter.Quote(kv.Value))
                        .Append(",\"x\":").Append(F(kv.Key.x))
                        .Append(",\"y\":").Append(F(kv.Key.y))
                        .Append(",\"z\":").Append(F(kv.Key.z))
                        .Append('}');
                }
            }
            catch (Exception)
            {
            }
            sb.Append("],\"updated_at\":").Append(JsonWriter.Quote(DateTime.UtcNow.ToString("o"))).Append('}');
            _json = sb.ToString();
        }

        /// <summary>
        /// The ZDO table. Its accessor has moved between game versions (a
        /// private m_objectsByID field, later a GetObjectsByID method), so it is
        /// resolved by reflection once instead of pinning the plugin to one.
        /// </summary>
        private Dictionary<ZDOID, ZDO> ObjectsById(ZDOMan zdoman)
        {
            if (!_lookupResolved)
            {
                _lookupResolved = true;
                var t = typeof(ZDOMan);
                var flags = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;
                _objectsMethod = t.GetMethod("GetObjectsByID", flags, null, Type.EmptyTypes, null);
                _objectsField = t.GetField("m_objectsByID", flags);
            }
            object value = null;
            if (_objectsMethod != null) value = _objectsMethod.Invoke(zdoman, null);
            else if (_objectsField != null) value = _objectsField.GetValue(zdoman);
            return value as Dictionary<ZDOID, ZDO>;
        }

        private static string F(float v)
        {
            return Math.Round(v, 1).ToString(System.Globalization.CultureInfo.InvariantCulture);
        }
    }
}
