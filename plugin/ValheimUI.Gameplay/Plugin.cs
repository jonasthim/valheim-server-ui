using System;
using BepInEx;
using BepInEx.Configuration;
using HarmonyLib;
using UnityEngine;
using UnityEngine.Rendering;

namespace ValheimUI.Gameplay
{
    /// <summary>
    /// Valheim UI Gameplay: opt-in, server-side gameplay helpers for servers
    /// running Valheim Server UI — autofeed hungry tamed animals from
    /// sign-marked chests (G-2) and a filter that keeps chosen random events
    /// (raids) from starting on their own (this card). Installed and
    /// configured by the manager together with BepInEx; players need
    /// nothing installed. Its cfg reloads live: the manager rewrites the
    /// file, this plugin picks the change up within a second, no restart.
    /// </summary>
    [BepInPlugin(Guid, Name, BuildInfo.Version)]
    public sealed class GameplayPlugin : BaseUnityPlugin
    {
        public const string Guid = "se.jonasthim.valheimui.gameplay";
        public const string Name = "Valheim UI Gameplay";

        internal static BepInEx.Logging.ManualLogSource Log { get; private set; }

        // Bound here, read by the autofeed scan loop added in G-2.
        internal static ConfigEntry<bool> AutofeedEnabled { get; private set; }
        internal static ConfigEntry<string> AutofeedSignPrefix { get; private set; }
        internal static ConfigEntry<float> AutofeedSignRadius { get; private set; }
        internal static ConfigEntry<float> AutofeedRange { get; private set; }
        internal static ConfigEntry<int> AutofeedIntervalSeconds { get; private set; }

        private ConfigEntry<bool> _raidsEnabled;
        private ConfigEntry<string> _raidsDisabled;

        private void Awake()
        {
            AutofeedEnabled = Config.Bind("Autofeed", "Enabled", false,
                "Feed hungry tamed animals from nearby chests marked with a sign. Changes apply live.");
            AutofeedSignPrefix = Config.Bind("Autofeed", "SignPrefix", "feed",
                "Case-insensitive prefix of a sign's text that marks a chest as a feed chest. Empty marks every chest in range.");
            AutofeedSignRadius = Config.Bind("Autofeed", "SignRadius", 2.0f,
                new ConfigDescription("Maximum distance in metres between the sign and the chest.", new AcceptableValueRange<float>(0.5f, 6f)));
            AutofeedRange = Config.Bind("Autofeed", "Range", 20f,
                new ConfigDescription("Maximum distance in metres between a feed chest and the animal it feeds.", new AcceptableValueRange<float>(1f, 100f)));
            AutofeedIntervalSeconds = Config.Bind("Autofeed", "IntervalSeconds", 30,
                new ConfigDescription("Seconds between scans for hungry animals.", new AcceptableValueRange<int>(5, 600)));
            _raidsEnabled = Config.Bind("Raids", "Enabled", true,
                "Keep the raids listed in Disabled from starting on their own. Changes apply live.");
            _raidsDisabled = Config.Bind("Raids", "Disabled", "",
                "Comma-separated random event names that never start by themselves (e.g. army_eikthyr,wolves). The event command still starts them.");

            if (SystemInfo.graphicsDeviceType != GraphicsDeviceType.Null)
            {
                Logger.LogInfo("Valheim UI Gameplay is a dedicated-server plugin; not starting on a game client.");
                return;
            }
            Log = Logger;

            try
            {
                new Harmony(Guid).PatchAll(typeof(GameplayPlugin).Assembly);
            }
            catch (Exception e)
            {
                Logger.LogWarning("Raid filter patches failed to apply: " + e.Message);
            }

            ApplyRaidFilterConfig();
            // BepInEx raises this for any entry, including after Config.Reload();
            // re-applying on every change is cheap and keeps us from having to
            // track which specific entries moved.
            Config.SettingChanged += (_, __) => ApplyRaidFilterConfig();

            ConfigReload.Start(Config, Logger);
        }

        private void ApplyRaidFilterConfig()
        {
            RaidFilter.Configure(_raidsEnabled.Value, _raidsDisabled.Value);
        }

        private void Update()
        {
            try
            {
                ConfigReload.Poll();
            }
            catch (Exception e)
            {
                Logger.LogWarning("config reload poll failed: " + e.Message);
            }
        }

        private void OnDestroy()
        {
            try
            {
                ConfigReload.Stop();
            }
            catch (Exception e)
            {
                Logger.LogWarning("config reload stop failed: " + e.Message);
            }
        }
    }
}
