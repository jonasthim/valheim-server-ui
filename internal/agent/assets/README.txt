Release builds place valheim-ui-agent.zip (the server plugin built from
plugin/) in this directory before `go build`, so the manager can install the
agent offline. Development builds have no zip here and fetch the asset of the
matching GitHub release instead (internal/agent/bundle.go).
