/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Drag01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import type { CellContext, Row } from '@tanstack/react-table'
import { Reorder, useDragControls } from 'motion/react'
import { createContext, useContext, useRef, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import {
  DataTableRow,
  type DataTableRenderRowHelpers,
} from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import type { useChannelPriorityOrder } from '../hooks/use-channel-priority-order'
import { isTagAggregateRow } from '../lib'
import type { Channel } from '../types'

type PriorityOrder = ReturnType<typeof useChannelPriorityOrder>
type HandleProps = {
  id: number
  order: PriorityOrder
  disabledReason?: string
  instructionsId: string
}
const DragContext = createContext<{
  controls: ReturnType<typeof useDragControls>
  dragging: { current: boolean }
} | null>(null)
const ChannelPriorityContext = createContext<Omit<HandleProps, 'id'> | null>(
  null
)

export function ChannelPriorityProvider(
  props: Omit<HandleProps, 'id'> & { children: ReactNode }
) {
  return (
    <ChannelPriorityContext.Provider value={props}>
      {props.children}
    </ChannelPriorityContext.Provider>
  )
}

export function ChannelPriorityCell(props: CellContext<Channel, unknown>) {
  const context = useContext(ChannelPriorityContext)
  if (!context || isTagAggregateRow(props.row.original)) return null
  return <ChannelPriorityHandle id={props.row.original.id} {...context} />
}

function ChannelPriorityHandle(props: HandleProps) {
  const { t } = useTranslation()
  const drag = useContext(DragContext)
  const moving = props.order.activeId === props.id && !props.order.saving
  const blocked = Boolean(
    props.disabledReason || (props.order.activeId !== undefined && !moving)
  )
  return (
    <Button
      type='button'
      variant='ghost'
      size='icon-sm'
      className={cn(
        'text-muted-foreground touch-none',
        blocked
          ? 'cursor-not-allowed opacity-40'
          : 'cursor-grab active:cursor-grabbing'
      )}
      aria-label={t('Reorder channel {{id}}', { id: props.id })}
      aria-describedby={props.instructionsId}
      aria-disabled={blocked}
      aria-pressed={moving}
      title={
        props.disabledReason ||
        t(
          'Drag to reorder. Space selects; arrow keys move; Enter saves; Escape cancels.'
        )
      }
      onPointerDown={(event) => {
        if (blocked || event.button !== 0 || !drag) return
        event.currentTarget.focus()
        if (props.order.begin(props.id)) {
          drag.dragging.current = false
          event.currentTarget.setPointerCapture?.(event.pointerId)
          drag.controls.start(event)
        }
      }}
      onPointerUp={() => {
        if (!drag?.dragging.current) {
          drag?.controls.cancel()
          props.order.cancel()
        }
      }}
      onPointerCancel={() => {
        drag?.controls.cancel()
        props.order.cancel()
      }}
      onBlur={() => {
        if (moving) {
          drag?.controls.cancel()
          props.order.cancel()
        }
      }}
      onKeyDown={(event) => {
        if (event.key === 'Escape') {
          event.preventDefault()
          drag?.controls.cancel()
          props.order.cancel()
          return
        }
        if (blocked) return
        if (event.key === 'Enter') {
          event.preventDefault()
          if (moving) void props.order.commit()
          else props.order.begin(props.id)
          return
        }
        if (event.key === ' ') {
          // Space only picks up a channel; saving stays on Enter so a second
          // Space cannot accidentally commit the move.
          event.preventDefault()
          if (!moving) props.order.begin(props.id)
          return
        }
        if (moving && (event.key === 'ArrowUp' || event.key === 'ArrowDown')) {
          event.preventDefault()
          props.order.step(event.key === 'ArrowUp' ? -1 : 1)
          event.currentTarget.scrollIntoView?.({
            block: 'nearest',
            inline: 'nearest',
          })
        }
      }}
    >
      <HugeiconsIcon icon={Drag01Icon} aria-hidden='true' />
    </Button>
  )
}

export function ChannelPriorityRow(props: {
  row: Row<Channel>
  helpers: DataTableRenderRowHelpers
  order: PriorityOrder
  className?: string
}) {
  const controls = useDragControls()
  const dragging = useRef(false)
  return (
    <DragContext.Provider value={{ controls, dragging }}>
      <DataTableRow
        row={props.row}
        className={props.className}
        getColumnClassName={(id) => props.helpers.getCellClassName(id)}
        render={(rowProps) => (
          <Reorder.Item
            as='tr'
            value={props.row.original.id}
            data-slot='table-row'
            data-state={props.row.getIsSelected() ? 'selected' : undefined}
            className={cn(
              rowProps.className,
              'relative',
              props.order.activeId === props.row.original.id && 'bg-muted'
            )}
            dragListener={false}
            dragControls={controls}
            dragMomentum={false}
            layout='position'
            onDragStart={() => {
              dragging.current = true
            }}
            onDragEnd={(event, info) => {
              dragging.current = false
              const target = event.target as HTMLElement | null
              const bounds = target?.closest('tbody')?.getBoundingClientRect()
              const x = info.point.x - window.scrollX
              const y = info.point.y - window.scrollY
              if (
                !bounds ||
                x < bounds.left ||
                x > bounds.right ||
                y < bounds.top ||
                y > bounds.bottom
              ) {
                props.order.cancel()
                return
              }
              void props.order.commit()
            }}
          >
            {rowProps.children}
          </Reorder.Item>
        )}
      />
    </DragContext.Provider>
  )
}
