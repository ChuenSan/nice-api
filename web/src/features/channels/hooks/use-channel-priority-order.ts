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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { handleServerError } from '@/lib/handle-server-error'
import {
  getServerErrorSources,
  getServerErrorStatus,
  safeServerErrorMessage,
} from '@/lib/server-error-message'

import { reorderChannelPriority } from '../api'
import { useChannels } from '../components/channels-provider'
import { channelsQueryKeys } from '../lib'
import type { Channel, ReorderChannelPriorityParams } from '../types'

type Move = {
  movedId: number
  rows: Channel[]
  order: number[]
  snapshot: string
  view: string
  saving: boolean
}

type PriorityOrderOptions = {
  status?: ReorderChannelPriorityParams['status']
  channels: Channel[]
  snapshot?: string
  total: number
  page: number
  pageSize: number
  view: string
  disabledReason?: string
}

export function useChannelPriorityOrder(options: PriorityOrderOptions) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { beginPriorityReorder, endPriorityReorder } = useChannels()
  const active = useRef<Move | null>(null)
  const [preview, setPreview] = useState<Move | null>(null)
  const [message, setMessage] = useState('')
  const mutation = useMutation({
    mutationFn: reorderChannelPriority,
    retry: false,
    onError: () => {}, // The operation below owns rollback/reconciliation and feedback.
  })

  function cancel() {
    if (!active.current || active.current.saving) return
    active.current = null
    setPreview(null)
    endPriorityReorder()
    setMessage(t('Channel move cancelled.'))
  }

  useEffect(() => {
    return () => {
      if (active.current && !active.current.saving) {
        active.current = null
        setPreview(null)
        endPriorityReorder()
      }
    }
  }, [options.view, endPriorityReorder])

  function begin(movedId: number) {
    if (
      options.disabledReason ||
      !options.snapshot ||
      active.current ||
      !beginPriorityReorder()
    ) {
      return false
    }
    const move: Move = {
      movedId,
      rows: options.channels,
      order: options.channels.map((channel) => channel.id),
      snapshot: options.snapshot,
      view: options.view,
      saving: false,
    }
    active.current = move
    setPreview(move)
    setMessage(
      t(
        'Channel selected. Use arrow keys to move, Enter to save, or Escape to cancel.'
      )
    )
    return true
  }

  function reorder(order: number[]) {
    const move = active.current
    if (!move || move.saving) return
    const next = { ...move, order }
    active.current = next
    setPreview(next)
    setMessage(
      t('Channel moved to position {{position}} of {{count}} on this page.', {
        position: order.indexOf(move.movedId) + 1,
        count: order.length,
      })
    )
  }

  function step(direction: -1 | 1) {
    const move = active.current
    if (!move || move.saving) return
    const index = move.order.indexOf(move.movedId)
    const target = index + direction
    if (target < 0 || target >= move.order.length) return
    const order = [...move.order]
    order.splice(index, 1)
    order.splice(target, 0, move.movedId)
    reorder(order)
  }

  async function commit() {
    const move = active.current
    if (!move || move.saving) return
    const index = move.order.indexOf(move.movedId)
    const targetId = move.rows[index]?.id
    if (move.view !== options.view || !targetId || targetId === move.movedId) {
      cancel()
      return
    }
    move.saving = true
    setPreview({ ...move })
    setMessage(t('Saving channel priorities…'))
    let reload = false
    try {
      await mutation.mutateAsync({
        ...(options.status ? { status: options.status } : {}),
        expected_snapshot: move.snapshot,
        page: options.page,
        page_size: options.pageSize,
        moved_id: move.movedId,
        target_id: targetId,
      })
      reload = true
      setMessage(t('Channel priorities saved.'))
    } catch (error) {
      const status = getServerErrorStatus(error)
      const sources = getServerErrorSources(error)
      const conflict =
        status === 409 ||
        sources.some((source) => source.code === 'channel_order_conflict')
      const knownFailure =
        sources.some(
          (source) =>
            source.code === 'channel_order_failed' ||
            source.code === 'invalid_priority_move'
        ) ||
        (status !== undefined &&
          status >= 400 &&
          status < 500 &&
          status !== 409 &&
          status !== 408)
      reload = conflict || !knownFailure
      let feedback = t(
        'Could not save channel priorities. The previous order has been restored.'
      )
      if (conflict) {
        feedback = t(
          'Channel order changed. Loading the latest order; please move the channel again.'
        )
      } else if (reload) {
        feedback = t(
          'The save result is uncertain. Loading the latest channel order before another move.'
        )
      }
      setMessage(feedback)
      handleServerError({
        message: feedback,
        [safeServerErrorMessage]: true,
        cause: error,
      })
      if (!reload) setPreview(null)
    }
    if (reload) {
      try {
        await queryClient.invalidateQueries(
          { queryKey: channelsQueryKeys.lists() },
          { throwOnError: true }
        )
      } catch (error) {
        const feedback = t(
          'Could not load the latest channel order. Refresh before moving a channel again.'
        )
        setMessage(feedback)
        handleServerError({
          message: feedback,
          [safeServerErrorMessage]: true,
          cause: error,
        })
      }
    }
    active.current = null
    setPreview(null)
    endPriorityReorder()
  }

  const visible = preview?.view === options.view ? preview : null
  let channels = options.channels
  if (visible) {
    channels = visible.order.flatMap((id, index) => {
      const channel = visible.rows.find((row) => row.id === id)
      if (!channel) return []
      if (!visible.saving) return channel
      return {
        ...channel,
        priority: options.total - (options.page - 1) * options.pageSize - index,
      }
    })
  }
  return {
    channels,
    values: channels.map((channel) => channel.id),
    activeId: visible?.movedId,
    saving: Boolean(visible?.saving),
    message,
    begin,
    cancel,
    reorder,
    step,
    commit,
  }
}
