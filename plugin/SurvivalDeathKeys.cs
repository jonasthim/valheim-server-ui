namespace ValheimUI
{
    // Replicated on the dying player's ZDO. The client owns these values;
    // the server only uses them after observing Valheim's own dead flag.
    internal static class SurvivalDeathKeys
    {
        public const string Enemy = "valheimui.death.enemy";
        public const string EnemyLevel = "valheimui.death.enemy_level";
        public const string Situation = "valheimui.death.situation";
    }
}
