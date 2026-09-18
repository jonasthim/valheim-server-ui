// Map tab: the seed-rendered world map with live players from the agent and
// the objects the server knows about (portals, ships, carts, tombstones, beds,
// boss locations), each as a toggleable layer.
import { useState } from "react";
import {
  Alert,
  Badge,
  Button,
  Chip,
  Group,
  Loader,
  Progress,
  Skeleton,
  Stack,
  Text,
  Tooltip,
} from "@mantine/core";
import { modals } from "@mantine/modals";
import {
  IconAlertTriangle,
  IconMapOff,
  IconRefresh,
} from "@tabler/icons-react";
import { useAuth } from "../../auth/useAuth";
import { fmtAgo } from "../../lib/format";
import { LoadError, SectionCard, StatusPill } from "../../ui";
import { AgentSetupNotice, BroadcastButton, useAgentCommand } from "../agent";
import { MapPlayerList } from "./MapPlayerList";
import { MapView } from "./MapView";
import { DEFAULT_LAYERS, type Layer, useLiveMap } from "./useLiveMap";
import { useRenderMap } from "./useMap";

const LAYERS: { id: Layer; label: string }[] = [
  { id: "players", label: "Players" },
  { id: "pins", label: "Pins" },
  { id: "portals", label: "Portals" },
  { id: "locations", label: "Bosses & places" },
  { id: "ships", label: "Ships" },
  { id: "carts", label: "Carts" },
  { id: "tombstones", label: "Tombstones" },
  { id: "beds", label: "Beds" },
];

const ANIMATE_KEY = "map.animate";

/** Animation preference: remembered per browser, off for reduced-motion users. */
function initialAnimate(): boolean {
  try {
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches)
      return false;
    const v = localStorage.getItem(ANIMATE_KEY);
    return v === null ? true : v === "1";
  } catch {
    return true;
  }
}

export function MapTab({ id }: { id: string }) {
  const { hasRole } = useAuth();
  const render = useRenderMap(id);
  const command = useAgentCommand(id);
  const [layers, setLayers] = useState<string[]>(DEFAULT_LAYERS);
  const [fog, setFog] = useState(true);
  const [animate, setAnimateState] = useState(initialAnimate);
  const [imageBroken, setImageBroken] = useState(false);
  function setAnimate(on: boolean) {
    setAnimateState(on);
    try {
      localStorage.setItem(ANIMATE_KEY, on ? "1" : "0");
    } catch {
      // storage unavailable: the choice lasts for this page only
    }
  }

  const {
    map,
    data,
    agent,
    explored,
    players,
    fogSupported,
    canLiftFog,
    fogOn,
    tiles,
    overlays,
    pings,
    markers,
    imageUrl,
  } = useLiveMap(id, { fog, layers, animate });

  const info = data?.info;
  const rendering = info?.state === "rendering" || info?.state === "encoding";
  const showImage = !!data?.image_ready && !imageBroken;
  const hidden = players.filter((p) => !p.position).length;

  function confirmRerender() {
    modals.openConfirmModal({
      title: "Re-render the map",
      children: (
        <Text size="sm">
          The server samples the whole world again in the background (a minute
          or two at the default resolution). Players are not affected.
        </Text>
      ),
      labels: { confirm: "Re-render", cancel: "Cancel" },
      onConfirm: () => render.mutate({ force: true }),
    });
  }

  if (map.isLoading) {
    return <Skeleton height={480} />;
  }
  if (!data)
    return (
      <LoadError
        error={map.error}
        title="Could not load the map"
        onRetry={() => map.refetch()}
      />
    );

  let overlay: React.ReactNode = null;
  if (!showImage) {
    overlay = (
      <Stack
        align="center"
        gap="xs"
        p="lg"
        style={{
          background: "rgba(10,12,18,0.7)",
          borderRadius: 12,
          maxWidth: 360,
        }}
      >
        {rendering ? (
          <>
            <Loader size="sm" />
            <Text size="sm" c="white">
              Rendering the world map on the server…{" "}
              {Math.round((info?.progress ?? 0) * 100)}%
            </Text>
            <Progress value={(info?.progress ?? 0) * 100} w="100%" size="sm" />
          </>
        ) : (
          <>
            {!agent.data?.installed || (data.connected && !data.map_supported) ? (
              <AgentSetupNotice id={id} context="map" compact />
            ) : (
              <>
                <IconMapOff size={28} color="var(--vh-text-soft)" />
                <Text size="sm" c="white" ta="center">
                  {!data.connected
                    ? "No map yet. Start the server with the agent; it renders the map a few seconds after the world loads."
                    : info?.state === "failed"
                      ? `The render failed: ${info.error || "unknown error"}`
                      : "Waiting for the server to start the render…"}
                </Text>
              </>
            )}
          </>
        )}
      </Stack>
    );
  }

  return (
    <Stack gap="md">
      <SectionCard
        title="World map"
        description="The world drawn like the in-game map from the server's own sampling, players live. Scroll to zoom, drag to pan."
        actions={
          <Group gap="sm" wrap="wrap" justify="flex-end">
            <StatusPill color={data.connected ? "moss" : "gray"}>
              {data.connected ? "live" : "agent offline"}
            </StatusPill>
            {data.stale && (
              <Badge
                color="orange"
                variant="light"
                leftSection={<IconAlertTriangle size={12} />}
              >
                cached image
              </Badge>
            )}
            {info && info.state === "ready" && (
              <Badge variant="outline" color="gray">
                {info.size} px, seed {info.seed}
              </Badge>
            )}
            {hasRole("operator") && data.connected && data.map_supported && (
              <Button
                size="xs"
                variant="light"
                leftSection={<IconRefresh size={14} />}
                loading={render.isPending}
                disabled={rendering}
                onClick={confirmRerender}
              >
                Re-render
              </Button>
            )}
            {hasRole("operator") && (
              <BroadcastButton id={id} disabled={!data.connected} />
            )}
          </Group>
        }
      >
        <Stack gap="sm">
          <Group gap={6} justify="space-between" wrap="wrap">
            <Chip.Group multiple value={layers} onChange={setLayers}>
              <Group gap={6}>
                {LAYERS.map((l) => (
                  <Chip key={l.id} value={l.id} size="xs" variant="light">
                    {l.label}
                  </Chip>
                ))}
              </Group>
            </Chip.Group>
            <Group gap={6}>
              <Tooltip label="Drifting fog, water shimmer and pings, as on the in-game map">
                <div>
                  <Chip
                    checked={animate}
                    onChange={setAnimate}
                    size="xs"
                    variant="filled"
                    color="iron"
                  >
                    Animate
                  </Chip>
                </div>
              </Tooltip>
              <Tooltip
                label={
                  !fogSupported
                    ? "The running agent has no exploration tracking; update it from the Mods tab"
                    : canLiftFog
                      ? "Only terrain players have explored (or shared on a cartography table) is shown. Operators can lift the fog."
                      : "Only terrain players have explored (or shared on a cartography table) is shown"
                }
              >
                <div>
                  <Chip
                    checked={fogOn}
                    onChange={setFog}
                    size="xs"
                    variant="filled"
                    color="iron"
                    disabled={!fogSupported || !canLiftFog}
                  >
                    Fog of war
                    {explored
                      ? `, ${explored.percent.toFixed(1)}% explored`
                      : ""}
                  </Chip>
                </div>
              </Tooltip>
            </Group>
          </Group>
          <MapView
            imageUrl={imageUrl}
            tiles={tiles}
            markers={markers}
            overlays={overlays}
            pings={pings}
            overlay={overlay}
            onImageError={() => setImageBroken(true)}
            onImageLoad={() => setImageBroken(false)}
          />
          <Group justify="space-between" gap="xs" wrap="wrap">
            <Text size="xs" c="dimmed">
              {players.length} player{players.length === 1 ? "" : "s"} online
              {hidden > 0 ? `, ${hidden} hiding their position` : ""},{" "}
              {data.objects.length} objects
              {data.objects_updated_at
                ? ` (scanned ${fmtAgo(data.objects_updated_at)})`
                : ""}
            </Text>
            <Text size="xs" c="dimmed">
              Beyond the 10 km circle lies the edge of the world.
            </Text>
          </Group>
          {data.stale && !data.connected && (
            <Alert
              color="orange"
              variant="light"
              icon={<IconAlertTriangle size={16} />}
            >
              This image was rendered during an earlier run. Start the server to
              confirm it still matches the world.
            </Alert>
          )}
          {data.connected && data.map_supported && !data.layers_supported && (
            <AgentSetupNotice id={id} context="map" compact />
          )}
        </Stack>
      </SectionCard>
      <MapPlayerList
        id={id}
        players={players}
        connected={data.connected}
        onKick={
          hasRole("operator")
            ? (name) => command.mutate({ command: "kick", target: name })
            : undefined
        }
      />
    </Stack>
  );
}
