using System;
using System.Collections.Generic;
using System.Reflection;
using UnityEngine;

namespace ValheimUI.Gameplay
{
    /// <summary>
    /// Server-side ZDO access helpers for the autofeed scan: a version-tolerant
    /// accessor for ZDOMan's object table and a per-prefab-hash component
    /// cache, so one scan pays for one <see cref="ZNetScene"/> lookup per
    /// distinct prefab, not per object. Copied from the agent's
    /// <c>MapObjects.ObjectsById</c> pattern on purpose — the two plugins are
    /// separate assemblies and share no code (plan Global rule 1).
    /// </summary>
    internal static class ZdoAccess
    {
        private static bool _lookupResolved;
        private static FieldInfo _objectsField;
        private static MethodInfo _objectsMethod;

        /// <summary>
        /// The ZDO table. Its accessor has moved between game versions (a
        /// private m_objectsByID field, later a GetObjectsByID method), so it
        /// is resolved by reflection once instead of pinning the plugin to
        /// one (verified against assembly_valheim.dll, scratchpad/p0/ZDOMan.cs
        /// line 91: <c>private readonly Dictionary&lt;ZDOID, ZDO&gt;
        /// m_objectsByID</c>).
        /// </summary>
        public static Dictionary<ZDOID, ZDO> ObjectsById(ZDOMan zdoman)
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

        private static readonly Dictionary<int, GameObject> PrefabByHash = new Dictionary<int, GameObject>();
        private static readonly Dictionary<int, Dictionary<Type, Component>> ComponentByHash = new Dictionary<int, Dictionary<Type, Component>>();

        /// <summary>
        /// Resolves the prefab GameObject for a ZDO's prefab hash through
        /// <see cref="ZNetScene"/>, caching the result including a negative
        /// one (unknown prefab hash) once the scene is actually loaded. Not
        /// cached while <see cref="ZNetScene.instance"/> is still null, so an
        /// early scan (before the world finishes loading) retries instead of
        /// freezing in a permanent miss.
        /// </summary>
        private static GameObject Prefab(int hash)
        {
            GameObject go;
            if (PrefabByHash.TryGetValue(hash, out go)) return go;
            var scene = ZNetScene.instance;
            if (scene == null) return null;
            go = scene.GetPrefab(hash);
            PrefabByHash[hash] = go;
            return go;
        }

        /// <summary>
        /// The prefab's component of type T, cached per (hash, type)
        /// including negative results, so a scan does one
        /// GetComponent&lt;T&gt; per prefab, not per object.
        /// </summary>
        public static T PrefabComponent<T>(int hash) where T : Component
        {
            var go = Prefab(hash);
            if (go == null) return null;

            Dictionary<Type, Component> byType;
            if (!ComponentByHash.TryGetValue(hash, out byType))
            {
                byType = new Dictionary<Type, Component>();
                ComponentByHash[hash] = byType;
            }

            var type = typeof(T);
            Component comp;
            if (byType.TryGetValue(type, out comp)) return (T)comp;

            comp = go.GetComponent<T>();
            byType[type] = comp; // caches null too (negative result)
            return (T)comp;
        }

        /// <summary>Prefab display name for debug logging; falls back to the hash when the prefab is unknown.</summary>
        public static string PrefabName(int hash)
        {
            var go = Prefab(hash);
            return go != null ? go.name : "#" + hash;
        }
    }
}
