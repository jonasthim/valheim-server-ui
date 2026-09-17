// A live in-game chat panel: shouts and normal messages from the running
// world, stored server-side (GET /instances/{id}/chat) and pushed live over
// the agent.chat SSE event, with a search box over the history and a
// composer that sends a server message through the `say` command. Whispers
// never reach the agent, so they never appear here.
import { useEffect, useRef, useState } from 'react'
import { Badge, Button, Group, Modal, ScrollArea, Stack, Text, TextInput } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { IconMessage, IconSearch } from '@tabler/icons-react'
import { fmtAgo } from '../../lib/format'
import { useAgentChat, useAgentCommand } from './useAgent'

export function ChatButton({ id, disabled, canSay }: { id: string; disabled?: boolean; canSay: boolean }) {
  const [opened, { open, close }] = useDisclosure(false)
  const [searchInput, setSearchInput] = useState('')
  const [search, setSearch] = useState('')
  const chat = useAgentChat(id, opened, search)
  const command = useAgentCommand(id)
  const [text, setText] = useState('')
  const viewport = useRef<HTMLDivElement>(null)

  // The API returns newest-first; a chat feed reads top-to-bottom oldest-first.
  const messages = [...(chat.data?.entries ?? [])].reverse()

  // Keep the newest message in view as the feed grows.
  useEffect(() => {
    if (opened) viewport.current?.scrollTo({ top: viewport.current.scrollHeight })
  }, [opened, messages.length])

  function send() {
    const msg = text.trim()
    if (!msg) return
    command.mutate(
      { command: 'say', message: msg },
      { onSuccess: (res) => res.ok && setText('') },
    )
  }

  return (
    <>
      <Button size="xs" variant="light" leftSection={<IconMessage size={14} />} onClick={open} disabled={disabled}>
        Chat
      </Button>
      <Modal opened={opened} onClose={close} title="In-game chat" centered size="lg">
        <Stack gap="sm">
          <TextInput
            placeholder="Search chat history"
            leftSection={<IconSearch size={14} />}
            value={searchInput}
            onChange={(e) => setSearchInput(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') setSearch(searchInput.trim())
            }}
          />
          <ScrollArea h={360} viewportRef={viewport} type="auto">
            <Stack gap={8} pr="sm">
              {messages.length === 0 && (
                <Text size="sm" c="dimmed">
                  {chat.isLoading ? 'Loading…' : search ? 'No chat matches your search.' : 'No chat yet. Shouts and messages from players show up here.'}
                </Text>
              )}
              {messages.map((m) => (
                <Group key={m.id} gap="xs" align="baseline" wrap="nowrap">
                  {m.type === 'shout' && (
                    <Badge size="xs" variant="light" color="straw">
                      shout
                    </Badge>
                  )}
                  <Text size="sm" fw={600} style={{ whiteSpace: 'nowrap' }}>
                    {m.sender}
                  </Text>
                  <Text size="sm" style={{ flex: 1, wordBreak: 'break-word' }}>
                    {m.text}
                  </Text>
                  <Text size="xs" c="dimmed" style={{ whiteSpace: 'nowrap' }}>
                    {fmtAgo(m.at)}
                  </Text>
                </Group>
              ))}
            </Stack>
          </ScrollArea>
          {canSay && (
            <Group gap="sm" wrap="nowrap" align="flex-end">
              <TextInput
                placeholder="Message players as the server"
                maxLength={200}
                value={text}
                onChange={(e) => setText(e.currentTarget.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') send()
                }}
                style={{ flex: 1 }}
                data-autofocus
              />
              <Button onClick={send} loading={command.isPending && command.variables?.command === 'say'} disabled={!text.trim()}>
                Send
              </Button>
            </Group>
          )}
        </Stack>
      </Modal>
    </>
  )
}
