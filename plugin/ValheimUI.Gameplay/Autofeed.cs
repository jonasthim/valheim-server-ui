using System;
using System.Collections.Generic;
using System.Diagnostics;

namespace ValheimUI.Gameplay
{
    /// <summary>
    /// Feeds hungry tamed animals from chests marked with a sign, entirely on
    /// the ZDOs the server owns the authoritative copy of (tamed animals
    /// near players are simulated on the players' clients, so the server
    /// cannot call <c>MonsterAI</c>/<c>Tameable</c> methods on them directly
    /// — see docs/ARCHITECTURE.md §21 and the plan's Design section).
    ///
    /// <see cref="Step"/>, called from <see cref="GameplayPlugin"/>'s
    /// <c>Update()</c>, drives a small state machine:
    ///   <list type="bullet">
    ///   <item><b>Idle</b>: waits until Enabled and IntervalSeconds have
    ///   elapsed since the last scan started, then begins one.</item>
    ///   <item><b>Scanning</b>: walks a one-time snapshot of every ZDO (a
    ///   live <c>Dictionary&lt;ZDOID,ZDO&gt;</c> is not safe to enumerate
    ///   across frames while the game mutates it) in slices bounded by
    ///   <see cref="ScanBudgetMs"/> per <c>Update()</c> call — the same
    ///   pattern as the agent's <c>MapRenderer.Step(budgetMs)</c> — bucketing
    ///   each ZDO into signs, feed-candidate chests and hungry-capable tamed
    ///   animals as it goes.</item>
    ///   <item>Once the snapshot is exhausted, <b>Match</b> (which chests
    ///   count as "feed chests", which tames are hungry) and <b>Feed</b>
    ///   (actually move items and reset feeding timers, capped at
    ///   <see cref="MaxFeedsPerScan"/>) run synchronously in that same call,
    ///   then the state returns to Idle.</item>
    ///   </list>
    /// Never writes a ZDO outside the Feed step; never touches a ZDO whose
    /// prefab is unknown (<see cref="ZdoAccess.PrefabComponent{T}"/> returns
    /// null for those, and every classification/match step treats null as
    /// "not this kind of thing" rather than guessing).
    /// </summary>
    internal static class Autofeed
    {
        /// <summary>Per-call budget for the Scan phase, matching the agent's map renderer convention.</summary>
        private const float ScanBudgetMs = 2.0f;

        /// <summary>Safety valve: a scan feeds at most this many animals, however many are hungry.</summary>
        private const int MaxFeedsPerScan = 200;

        private enum State
        {
            Idle,
            Scanning,
        }

        private sealed class ChestLoad
        {
            public ZDO Zdo;
            public Inventory Inventory;
        }

        private static bool _enabled;
        private static string _signPrefix = "";
        private static float _signRadius = 2f;
        private static float _range = 20f;
        private static int _intervalSeconds = 30;
        private static bool _debug;

        private static State _state = State.Idle;
        private static DateTime _lastScanStarted = DateTime.MinValue;
        private static ZDO[] _snapshot;
        private static int _scanIndex;
        private static readonly List<ZDO> Signs = new List<ZDO>();
        private static readonly List<ZDO> Chests = new List<ZDO>();
        private static readonly List<ZDO> Tames = new List<ZDO>();
        private static readonly HashSet<string> WarnedMessages = new HashSet<string>();

        /// <summary>Applies new [Autofeed] values; re-read on every config change (G-1's SettingChanged handler). Safe to call every time, even mid-scan.</summary>
        public static void Configure(bool enabled, string signPrefix, float signRadius, float range, int intervalSeconds, bool debug)
        {
            try
            {
                _enabled = enabled;
                _signPrefix = signPrefix ?? "";
                _signRadius = signRadius;
                _range = range;
                _intervalSeconds = intervalSeconds > 0 ? intervalSeconds : 1;
                _debug = debug;
                if (!enabled) ResetScan();
            }
            catch (Exception e)
            {
                LogWarningOnce("autofeed configure", e);
            }
        }

        private static void ResetScan()
        {
            _state = State.Idle;
            _snapshot = null;
            _scanIndex = 0;
            Signs.Clear();
            Chests.Clear();
            Tames.Clear();
        }

        /// <summary>Called every <c>Update()</c>. Idle, or continuing a scan already in flight.</summary>
        public static void Step()
        {
            try
            {
                if (!_enabled) return;
                var znet = ZNet.instance;
                if (znet == null || !znet.IsServer()) return;

                if (_state == State.Idle)
                {
                    if ((DateTime.UtcNow - _lastScanStarted).TotalSeconds < _intervalSeconds) return;
                    BeginScan();
                    if (_state != State.Scanning) return; // ZDOMan not ready yet; try again next Update()
                }

                ContinueScan();
            }
            catch (Exception e)
            {
                LogWarningOnce("autofeed step", e);
                ResetScan();
            }
        }

        private static void BeginScan()
        {
            var zdoman = ZDOMan.instance;
            if (zdoman == null) return;
            var table = ZdoAccess.ObjectsById(zdoman);
            if (table == null) return;

            _snapshot = new ZDO[table.Count];
            table.Values.CopyTo(_snapshot, 0);
            _scanIndex = 0;
            Signs.Clear();
            Chests.Clear();
            Tames.Clear();
            _state = State.Scanning;
            _lastScanStarted = DateTime.UtcNow;
        }

        private static void ContinueScan()
        {
            if (_snapshot == null)
            {
                _state = State.Idle;
                return;
            }

            var sw = Stopwatch.StartNew();
            while (_scanIndex < _snapshot.Length && sw.Elapsed.TotalMilliseconds < ScanBudgetMs)
            {
                ClassifyOne(_snapshot[_scanIndex]);
                _scanIndex++;
            }

            if (_scanIndex >= _snapshot.Length)
            {
                _snapshot = null;
                MatchAndFeed();
                _state = State.Idle;
            }
        }

        /// <summary>
        /// Buckets one ZDO by its prefab's components: Sign (non-empty text),
        /// Container (not <see cref="ZDOVars.s_inUse"/>), or a tamed
        /// (<see cref="ZDOVars.s_tamed"/>) animal whose prefab has both
        /// Tameable and a MonsterAI with at least one consumable item.
        /// </summary>
        private static void ClassifyOne(ZDO zdo)
        {
            try
            {
                if (zdo == null) return;
                int prefab = zdo.GetPrefab();

                if (ZdoAccess.PrefabComponent<Sign>(prefab) != null)
                {
                    var text = zdo.GetString(ZDOVars.s_text, "");
                    if (!string.IsNullOrEmpty(text)) Signs.Add(zdo);
                    return;
                }

                var container = ZdoAccess.PrefabComponent<Container>(prefab);
                if (container != null)
                {
                    if (zdo.GetInt(ZDOVars.s_inUse, 0) == 0) Chests.Add(zdo);
                    return;
                }

                if (!zdo.GetBool(ZDOVars.s_tamed, false)) return;
                var tameable = ZdoAccess.PrefabComponent<Tameable>(prefab);
                var monsterAI = ZdoAccess.PrefabComponent<MonsterAI>(prefab);
                if (tameable != null && monsterAI != null && monsterAI.m_consumeItems != null && monsterAI.m_consumeItems.Count > 0)
                {
                    Tames.Add(zdo);
                }
            }
            catch (Exception e)
            {
                LogWarningOnce("autofeed classify", e);
            }
        }

        /// <summary>Chests within SignRadius of a sign whose trimmed text starts with SignPrefix (OrdinalIgnoreCase); every chest when SignPrefix is empty.</summary>
        private static List<ZDO> FindFeedChests()
        {
            var result = new List<ZDO>(Chests.Count);
            if (_signPrefix.Length == 0)
            {
                result.AddRange(Chests);
                return result;
            }

            float radiusSq = _signRadius * _signRadius;
            foreach (var chest in Chests)
            {
                var chestPos = chest.GetPosition();
                bool matched = false;
                foreach (var sign in Signs)
                {
                    if ((sign.GetPosition() - chestPos).sqrMagnitude > radiusSq) continue;
                    var text = sign.GetString(ZDOVars.s_text, "");
                    if (text != null && text.Trim().StartsWith(_signPrefix, StringComparison.OrdinalIgnoreCase))
                    {
                        matched = true;
                        break;
                    }
                }
                if (matched) result.Add(chest);
            }
            return result;
        }

        /// <summary>Tames whose feeding timer is older than their own <see cref="Tameable.m_fedDuration"/> — the game's own IsHungry() rule, computed from the ZDO since the live component is not available server-side.</summary>
        private static List<ZDO> FindHungryTames()
        {
            var result = new List<ZDO>(Tames.Count);
            var znet = ZNet.instance;
            if (znet == null) return result;
            var now = znet.GetTime();

            foreach (var tame in Tames)
            {
                var tameable = ZdoAccess.PrefabComponent<Tameable>(tame.GetPrefab());
                if (tameable == null) continue;
                var lastFeeding = tame.GetLong(ZDOVars.s_tameLastFeeding, 0L);
                if ((now - new DateTime(lastFeeding)).TotalSeconds > tameable.m_fedDuration)
                {
                    result.Add(tame);
                }
            }
            return result;
        }

        /// <summary>Match + Feed: for each hungry tame, the nearest feed chest in Range holding a matching item loses one, the tame's feeding timer resets, both ZDOs are force-sent. One item per animal per scan; a chest's inventory loads once and saves once even if several animals eat from it.</summary>
        private static void MatchAndFeed()
        {
            var feedChests = FindFeedChests();
            var hungryTames = FindHungryTames();

            if (feedChests.Count == 0 || hungryTames.Count == 0)
            {
                LogScanSummary(0, 0, feedChests.Count, hungryTames.Count);
                return;
            }

            var zdoman = ZDOMan.instance;
            if (zdoman == null) return;

            // The scan spans several frames: an object destroyed meanwhile
            // may have been released to the ZDO pool and recycled, so only
            // objects the world table still maps to the same ZDO are used.
            feedChests.RemoveAll(z => !IsLive(zdoman, z));
            hungryTames.RemoveAll(z => !IsLive(zdoman, z));
            if (feedChests.Count == 0 || hungryTames.Count == 0)
            {
                LogScanSummary(0, 0, feedChests.Count, hungryTames.Count);
                return;
            }

            var loaded = new Dictionary<ZDOID, ChestLoad>();
            var usedChests = new HashSet<ZDOID>();
            float rangeSq = _range * _range;
            int fed = 0;

            foreach (var tame in hungryTames)
            {
                if (fed >= MaxFeedsPerScan) break;

                var monsterAI = ZdoAccess.PrefabComponent<MonsterAI>(tame.GetPrefab());
                if (monsterAI == null || monsterAI.m_consumeItems == null || monsterAI.m_consumeItems.Count == 0) continue;

                var tamePos = tame.GetPosition();
                ChestLoad best = null;
                float bestDistSq = float.MaxValue;
                ItemDrop.ItemData bestItem = null;

                foreach (var chestZdo in feedChests)
                {
                    float distSq = (chestZdo.GetPosition() - tamePos).sqrMagnitude;
                    if (distSq > rangeSq) continue;
                    if (best != null && distSq >= bestDistSq) continue;

                    ChestLoad load;
                    if (!loaded.TryGetValue(chestZdo.m_uid, out load))
                    {
                        load = LoadChest(chestZdo);
                        loaded[chestZdo.m_uid] = load; // cached even on failure so a bad chest is not retried this scan
                    }
                    if (load == null) continue;

                    var item = FindConsumable(load.Inventory, monsterAI.m_consumeItems);
                    if (item == null) continue;

                    best = load;
                    bestDistSq = distSq;
                    bestItem = item;
                }

                if (best == null || bestItem == null) continue;

                best.Inventory.RemoveOneItem(bestItem);
                tame.Set(ZDOVars.s_tameLastFeeding, ZNet.instance.GetTime().Ticks);
                zdoman.ForceSendZDO(tame.m_uid);
                usedChests.Add(best.Zdo.m_uid);
                fed++;

                if (_debug)
                {
                    GameplayPlugin.Log?.LogInfo("autofeed debug: fed " + ZdoAccess.PrefabName(tame.GetPrefab()) +
                        " from " + ZdoAccess.PrefabName(best.Zdo.GetPrefab()) +
                        " at " + Math.Sqrt(bestDistSq).ToString("F1") + "m");
                }
            }

            foreach (var kv in loaded)
            {
                if (kv.Value == null || !usedChests.Contains(kv.Key)) continue;
                SaveChest(kv.Value);
                zdoman.ForceSendZDO(kv.Key);
            }

            LogScanSummary(fed, usedChests.Count, feedChests.Count, hungryTames.Count);
        }

        /// <summary>True while the world table still maps the ZDO's id to this very object (not destroyed, not recycled).</summary>
        private static bool IsLive(ZDOMan zdoman, ZDO zdo)
        {
            try
            {
                return zdo != null && ReferenceEquals(zdoman.GetZDO(zdo.m_uid), zdo);
            }
            catch (Exception)
            {
                return false;
            }
        }

        private static ChestLoad LoadChest(ZDO zdo)
        {
            try
            {
                var container = ZdoAccess.PrefabComponent<Container>(zdo.GetPrefab());
                if (container == null) return null;
                var bytes = zdo.GetByteArray(ZDOVars.s_items);
                if (bytes == null) return null;

                var inventory = new Inventory("autofeed", null, container.m_width, container.m_height);
                inventory.Load(new ZPackage(bytes));
                return new ChestLoad { Zdo = zdo, Inventory = inventory };
            }
            catch (Exception e)
            {
                LogWarningOnce("autofeed chest load", e);
                return null;
            }
        }

        private static void SaveChest(ChestLoad load)
        {
            try
            {
                var pkg = new ZPackage();
                load.Inventory.Save(pkg);
                load.Zdo.Set(ZDOVars.s_items, pkg.GetArray());
            }
            catch (Exception e)
            {
                LogWarningOnce("autofeed chest save", e);
            }
        }

        private static ItemDrop.ItemData FindConsumable(Inventory inventory, List<ItemDrop> consumeItems)
        {
            foreach (var item in inventory.GetAllItems())
            {
                if (item == null || item.m_shared == null) continue;
                for (int i = 0; i < consumeItems.Count; i++)
                {
                    var consumable = consumeItems[i];
                    if (consumable == null || consumable.m_itemData == null || consumable.m_itemData.m_shared == null) continue;
                    if (consumable.m_itemData.m_shared.m_name == item.m_shared.m_name) return item;
                }
            }
            return null;
        }

        private static void LogScanSummary(int fed, int chestsUsed, int feedChests, int hungryTames)
        {
            if (fed > 0)
            {
                GameplayPlugin.Log?.LogInfo("autofeed: fed " + fed + " animal(s) from " + chestsUsed + " chest(s) (" +
                    feedChests + " feed chests, " + hungryTames + " hungry)");
            }
            else
            {
                GameplayPlugin.Log?.LogDebug("autofeed: " + feedChests + " feed chests, " + hungryTames + " hungry, 0 fed");
            }
        }

        private static void LogWarningOnce(string context, Exception e)
        {
            var message = context + ": " + e.Message;
            if (WarnedMessages.Add(message))
            {
                GameplayPlugin.Log?.LogWarning(message);
            }
        }
    }
}
