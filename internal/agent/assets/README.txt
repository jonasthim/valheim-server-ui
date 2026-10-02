Release builds place valheim-ui-agent.zip and valheim-ui-gameplay.zip (the two
server plugins built from plugin/) in this directory before `go build`, so the
manager can install them offline. Development builds have no zips here and
fetch the matching GitHub release asset instead (internal/agent/bundle.go).

For local development, VALHEIM_UI_AGENT_ZIP and VALHEIM_UI_GAMEPLAY_ZIP point
the manager at a locally built zip instead (plugin/dist/*.zip from
plugin/build.sh), skipping both the embedded copy and the release download.
