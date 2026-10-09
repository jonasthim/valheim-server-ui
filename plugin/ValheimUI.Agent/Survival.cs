using System;
using HarmonyLib;

namespace ValheimUI.Agent
{
    // Player.OnDeath runs on the owning client. Its OnDeath RPC is broadcast to
    // every peer, including the dedicated server, and carries no hit details.
    [HarmonyPatch(typeof(Player), "RPC_OnDeath")]
    internal static class SurvivalDeathPatch
    {
        private static void Postfix(Player __instance)
        {
            try { AgentPlugin.Instance?.RecordDeath(__instance); }
            catch (Exception e) { AgentPlugin.Log?.LogWarning("death capture failed: " + e.Message); }
        }
    }
}
