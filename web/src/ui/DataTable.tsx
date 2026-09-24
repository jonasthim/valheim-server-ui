import type { CSSProperties, ReactNode } from 'react'
import { useState } from 'react'
import { Skeleton, Table, Text, VisuallyHidden } from '@mantine/core'
import { IconChevronDown, IconChevronUp, IconSelector } from '@tabler/icons-react'
import classes from './DataTable.module.css'

export interface DataTableColumn<T> {
  key: string
  header: ReactNode
  render: (row: T) => ReactNode
  width?: number | string
  align?: 'left' | 'right' | 'center'
  nowrap?: boolean
  sticky?: boolean
  sortable?: boolean
  sortValue?: (row: T) => string | number
}

export interface DataTableProps<T> {
  columns: DataTableColumn<T>[]
  rows: T[]
  rowKey: (row: T, index: number) => string | number
  loading?: boolean
  loadingRows?: number
  empty?: ReactNode
  error?: ReactNode
  minWidth?: number
  stickyHeader?: boolean
  maxHeight?: CSSProperties['maxHeight']
  onRowClick?: (row: T) => void
  clickable?: (row: T) => boolean
  actions?: (row: T) => ReactNode
  defaultSort?: { key: string; dir: 'asc' | 'desc' }
  density?: 'compact' | 'default'
  'aria-label'?: string
}

type SortState = { key: string; dir: 'asc' | 'desc' }

/** localeCompare for strings, numeric otherwise. */
function compare(a: string | number, b: string | number): number {
  return typeof a === 'string' && typeof b === 'string' ? a.localeCompare(b) : Number(a) - Number(b)
}

function columnStyle<T>(col: DataTableColumn<T>): CSSProperties | undefined {
  if (col.width === undefined && !col.align && !col.nowrap) return undefined
  return { width: col.width, textAlign: col.align, whiteSpace: col.nowrap ? 'nowrap' : undefined }
}

/** The one `—` placeholder for an absent value in a `DataTable` cell (or anywhere else one is needed). */
export function Dash() {
  return (
    <Text span className={classes.dash}>
      —
    </Text>
  )
}

/**
 * Thin, typed composition over Mantine `Table`: sortable columns, skeleton
 * loading rows, an `EmptyState` empty row, an optional error row, a sticky
 * header inside a max-height scroll container, a sticky first column, row
 * click, and hover/focus-revealed trailing actions that stay visible on
 * touch and narrow screens. 36px rows, 32px header.
 */
export function DataTable<T>({
  columns,
  rows,
  rowKey,
  loading = false,
  loadingRows = 4,
  empty,
  error,
  minWidth,
  stickyHeader = false,
  maxHeight,
  onRowClick,
  clickable,
  actions,
  defaultSort,
  density = 'compact',
  'aria-label': ariaLabel,
}: DataTableProps<T>) {
  const [sort, setSort] = useState<SortState | undefined>(defaultSort)
  const colCount = columns.length + (actions ? 1 : 0)

  function toggleSort(col: DataTableColumn<T>) {
    if (!col.sortable) return
    setSort((prev) =>
      prev?.key === col.key ? { key: col.key, dir: prev.dir === 'asc' ? 'desc' : 'asc' } : { key: col.key, dir: 'asc' },
    )
  }

  let sortedRows = rows
  if (sort) {
    const sortColumn = columns.find((c) => c.key === sort.key)
    if (sortColumn?.sortValue) {
      const { sortValue } = sortColumn
      const dir = sort.dir === 'asc' ? 1 : -1
      sortedRows = [...rows].sort((a, b) => compare(sortValue(a), sortValue(b)) * dir)
    }
  }

  function renderBody() {
    if (loading) {
      return Array.from({ length: loadingRows }).map((_, i) => (
        <Table.Tr key={`skeleton-${i}`}>
          {columns.map((col) => (
            <Table.Td key={col.key}>
              <Skeleton height={14} width="60%" />
            </Table.Td>
          ))}
          {actions && <Table.Td className={classes.actionsCell} />}
        </Table.Tr>
      ))
    }
    if (error) {
      return (
        <Table.Tr>
          <Table.Td colSpan={colCount}>{error}</Table.Td>
        </Table.Tr>
      )
    }
    if (rows.length === 0) {
      return (
        <Table.Tr>
          <Table.Td colSpan={colCount} p="md">
            {empty}
          </Table.Td>
        </Table.Tr>
      )
    }
    return sortedRows.map((row, i) => {
      const isClickable = onRowClick ? (clickable ? clickable(row) : true) : false
      return (
        <Table.Tr
          key={rowKey(row, i)}
          onClick={isClickable ? () => onRowClick?.(row) : undefined}
          tabIndex={isClickable ? 0 : undefined}
          onKeyDown={
            isClickable
              ? (e) => {
                  if (e.key === 'Enter') onRowClick?.(row)
                }
              : undefined
          }
          className={isClickable ? classes.clickable : undefined}
        >
          {columns.map((col) => (
            <Table.Td key={col.key} className={col.sticky ? classes.stickyCol : undefined} style={columnStyle(col)}>
              {col.render(row)}
            </Table.Td>
          ))}
          {actions && (
            <Table.Td className={classes.actionsCell} onClick={(e) => e.stopPropagation()}>
              <div className={classes.actions}>{actions(row)}</div>
            </Table.Td>
          )}
        </Table.Tr>
      )
    })
  }

  return (
    <Table.ScrollContainer
      minWidth={minWidth ?? 480}
      maxHeight={stickyHeader ? (maxHeight ?? 'calc(100dvh - 240px)') : undefined}
      scrollAreaProps={{ viewportProps: { tabIndex: 0 } }}
    >
      <Table
        stickyHeader={stickyHeader}
        withRowBorders
        highlightOnHover
        tabularNums
        verticalSpacing={4}
        horizontalSpacing="md"
        data-density={density}
        classNames={{ table: classes.table, th: classes.th, td: classes.td, tr: classes.tr }}
        aria-label={ariaLabel}
      >
        <Table.Thead>
          <Table.Tr>
            {columns.map((col) => (
              <Table.Th
                key={col.key}
                className={col.sticky ? classes.stickyCol : undefined}
                style={columnStyle(col)}
                aria-sort={col.sortable ? (sort?.key === col.key ? (sort.dir === 'asc' ? 'ascending' : 'descending') : 'none') : undefined}
              >
                {col.sortable ? (
                  <button type="button" className={classes.sortButton} onClick={() => toggleSort(col)}>
                    {col.header}
                    {sort?.key === col.key ? (
                      sort.dir === 'asc' ? <IconChevronUp size={14} /> : <IconChevronDown size={14} />
                    ) : (
                      <IconSelector size={14} style={{ opacity: 0.5 }} />
                    )}
                  </button>
                ) : (
                  col.header
                )}
              </Table.Th>
            ))}
            {actions && (
              <Table.Th className={classes.actionsCell}>
                <VisuallyHidden>Actions</VisuallyHidden>
              </Table.Th>
            )}
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>{renderBody()}</Table.Tbody>
      </Table>
    </Table.ScrollContainer>
  )
}
