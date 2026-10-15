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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import type { Channel } from '../../types'
import { ChannelsPrimaryButtons } from '../channels-primary-buttons'
import { ChannelsProvider } from '../channels-provider'
import { ChannelsTable } from '../channels-table'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  initAsync: false,
})
const clients: QueryClient[] = []

function channel(name: string): Channel {
  return {
    id: 3,
    type: 1,
    key: '',
    status: 1,
    name,
    created_time: 0,
    test_time: 0,
    response_time: 0,
    other: '',
    balance: 0,
    balance_updated_time: 0,
    models: 'gpt-4o',
    group: 'vip',
    used_quota: 0,
    other_info: '',
    remark: '',
    max_input_tokens: 0,
    channel_info: {
      is_multi_key: false,
      multi_key_size: 0,
      multi_key_polling_index: 0,
      multi_key_mode: 'random',
    },
    settings: '{}',
  }
}

beforeEach(() => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  localStorage.clear()
})

afterEach(() => {
  cleanup()
  localStorage.clear()
  clients.splice(0).forEach((client) => client.clear())
  vi.restoreAllMocks()
})

async function renderPriorityPage(
  options: { search?: string; items?: Channel[]; snapshot?: string } = {}
) {
  const items = options.items ?? [
    { ...channel('D'), id: 4, priority: 10, tag: 'production' },
    { ...channel('E'), id: 3, priority: 10, tag: 'production' },
    { ...channel('F'), id: 2, priority: 5, status: 2 },
  ]
  const state = { items, snapshot: options.snapshot ?? 'baseline' }
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/channel' || url === '/api/channel/search') {
      return {
        data: {
          success: true,
          data: {
            items: [...state.items],
            total: state.items.length === 3 ? 6 : state.items.length,
            page: 2,
            page_size: 3,
            order_snapshot: state.snapshot,
          },
        },
      }
    }
    return { data: { success: true, data: [] } }
  })
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: 3 },
    },
  })
  clients.push(client)
  const root = createRootRoute()
  const auth = createRoute({ getParentRoute: () => root, id: '_authenticated' })
  const channelsRoute = createRoute({
    getParentRoute: () => auth,
    path: 'channels/',
    component: () => (
      <ChannelsProvider>
        <ChannelsPrimaryButtons />
        <ChannelsTable />
      </ChannelsProvider>
    ),
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([channelsRoute])]),
    history: createMemoryHistory({
      initialEntries: [options.search ?? '/channels/?page=2&pageSize=3'],
    }),
  })
  await router.load()
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </I18nextProvider>
  )
  await screen.findByRole('button', { name: 'Table view' })
  await userEvent.click(screen.getByRole('button', { name: 'Table view' }))
  await screen.findByRole('table')
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Refresh' })).toHaveAttribute(
      'aria-busy',
      'false'
    )
  )
  return { get, items, state }
}

function rowNames() {
  return within(screen.getByRole('table'))
    .getAllByRole('row')
    .slice(1)
    .map((row) =>
      within(row)
        .getByRole('button', { name: /^Reorder channel/ })
        .getAttribute('aria-label')
    )
}

it('saves a keyboard move on the second page optimistically with the snapshot and locks overlapping moves', async () => {
  let resolve!: (value: unknown) => void
  const post = vi.spyOn(api, 'post').mockImplementation(
    () =>
      new Promise((r) => {
        resolve = r
      }) as never
  )
  const { items } = await renderPriorityPage()
  const handle = screen.getByRole('button', { name: 'Reorder channel 2' })
  handle.focus()
  await userEvent.keyboard(' {ArrowUp}{ArrowUp}{Enter}')
  expect(rowNames()).toEqual([
    'Reorder channel 2',
    'Reorder channel 4',
    'Reorder channel 3',
  ])
  expect(post).toHaveBeenCalledWith(
    '/api/channel/priority/reorder',
    {
      expected_snapshot: 'baseline',
      page: 2,
      page_size: 3,
      moved_id: 2,
      target_id: 4,
    },
    expect.objectContaining({ skipAuthRefresh: true })
  )
  expect(handle).toHaveAttribute('aria-disabled', 'true')
  expect(handle).toHaveFocus()
  await userEvent.keyboard(' {ArrowDown}{Enter}')
  expect(post).toHaveBeenCalledTimes(1)
  items.splice(
    0,
    3,
    { ...items[2], priority: 3 },
    { ...items[0], priority: 2 },
    { ...items[1], priority: 1 }
  )
  resolve({ data: { success: true, data: { order_snapshot: 'saved' } } })
  await waitFor(() =>
    expect(screen.getByRole('status')).toHaveTextContent(
      'Channel priorities saved.'
    )
  )
  expect(rowNames()).toEqual([
    'Reorder channel 2',
    'Reorder channel 4',
    'Reorder channel 3',
  ])
})

it('restores the known previous order on a rejected save without retrying the write', async () => {
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: false, code: 'channel_order_failed' },
  })
  await renderPriorityPage()
  screen.getByRole('button', { name: 'Reorder channel 2' }).focus()
  await userEvent.keyboard(' {ArrowUp}{Enter}')
  await waitFor(() =>
    expect(screen.getByRole('status')).toHaveTextContent(
      'The previous order has been restored.'
    )
  )
  expect(rowNames()).toEqual([
    'Reorder channel 4',
    'Reorder channel 3',
    'Reorder channel 2',
  ])
  expect(
    screen.getByRole('button', { name: 'Reorder channel 2' })
  ).toHaveAttribute('aria-disabled', 'false')
  expect(post).toHaveBeenCalledTimes(1)
})

it.each([
  {
    label: '409 conflict',
    error: {
      response: { status: 409, data: { code: 'channel_order_conflict' } },
    },
    feedback: 'Channel order changed.',
  },
  {
    label: 'unknown transport result',
    error: new Error('connection lost'),
    feedback: 'The save result is uncertain.',
  },
])(
  'loads the latest order after a $label and never replays the write',
  async ({ error, feedback }) => {
    const post = vi.spyOn(api, 'post').mockRejectedValue(error)
    const { state, items } = await renderPriorityPage()
    state.items = [items[1], items[0], items[2]]
    state.snapshot = 'latest'
    screen.getByRole('button', { name: 'Reorder channel 2' }).focus()
    await userEvent.keyboard(' {ArrowUp}{ArrowUp}{Enter}')
    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent(feedback)
    )
    await waitFor(() =>
      expect(rowNames()).toEqual([
        'Reorder channel 3',
        'Reorder channel 4',
        'Reorder channel 2',
      ])
    )
    expect(post).toHaveBeenCalledTimes(1)
  }
)

it('requires a successful refresh after uncertain save reconciliation fails', async () => {
  const post = vi
    .spyOn(api, 'post')
    .mockRejectedValue(new Error('connection lost'))
  const { get } = await renderPriorityPage()
  get.mockRejectedValue(new Error('offline'))
  screen.getByRole('button', { name: 'Reorder channel 2' }).focus()
  await userEvent.keyboard(' {ArrowUp}{Enter}')
  await waitFor(() =>
    expect(screen.getByRole('status')).toHaveTextContent(
      'Could not load the latest channel order.'
    )
  )
  expect(
    screen.getByRole('button', { name: 'Reorder channel 2' })
  ).toHaveAttribute('aria-disabled', 'true')
  await userEvent.keyboard(' {ArrowUp}{Enter}')
  expect(post).toHaveBeenCalledTimes(1)
})

it.each([
  ' {ArrowUp}{Escape}',
  ' {ArrowUp}{ArrowDown}{Enter}',
  ' {ArrowDown}{Enter}',
])(
  'does not save a cancelled, original-position, or boundary move (%s)',
  async (keys) => {
    const post = vi.spyOn(api, 'post')
    await renderPriorityPage()
    screen.getByRole('button', { name: 'Reorder channel 2' }).focus()
    await userEvent.keyboard(keys)
    expect(post).not.toHaveBeenCalled()
    expect(rowNames()).toEqual([
      'Reorder channel 4',
      'Reorder channel 3',
      'Reorder channel 2',
    ])
    expect(
      screen.getByRole('button', { name: 'Reorder channel 2' })
    ).toHaveAttribute('aria-pressed', 'false')
  }
)

it.each([
  ['filter=production', 'Clear name and model searches'],
  ['model=gpt', 'Clear name and model searches'],
  ['status=%5B%22enabled%22%5D', 'Clear all channel filters'],
  ['type=%5B%221%22%5D', 'Clear all channel filters'],
  ['group=%5B%22vip%22%5D', 'Clear all channel filters'],
])(
  'disables handles with an accessible reason for the restricted view %s',
  async (search, reason) => {
    const post = vi.spyOn(api, 'post')
    await renderPriorityPage({ search: `/channels/?${search}` })
    const handle = screen.getByRole('button', { name: 'Reorder channel 2' })
    expect(handle).toHaveAttribute('aria-disabled', 'true')
    expect(handle).toHaveAccessibleDescription(expect.stringContaining(reason))
    handle.focus()
    await userEvent.keyboard(' {ArrowUp}{Enter}')
    expect(post).not.toHaveBeenCalled()
  }
)

it('disables moves immediately while model search input is awaiting its debounce', async () => {
  await renderPriorityPage()
  await userEvent.type(screen.getByPlaceholderText('Filter by model...'), 'gpt')
  expect(
    screen.getByRole('button', { name: 'Reorder channel 2' })
  ).toHaveAttribute('aria-disabled', 'true')
})

it('does not make aggregate tag rows draggable and explains how to return to individual channels', async () => {
  await renderPriorityPage()
  await userEvent.click(screen.getByRole('switch', { name: 'Tag Mode' }))
  expect(
    screen.getByText('Turn off tag aggregation to reorder individual channels.')
  ).toBeInTheDocument()
  const aggregate = (await screen.findByText('production')).closest('tr')
  if (!aggregate) throw new Error('Tag aggregate row not found')
  expect(
    within(aggregate).queryByRole('button', { name: /^Reorder channel/ })
  ).not.toBeInTheDocument()
})

it('requires priority descending instead of ID ordering', async () => {
  await renderPriorityPage()
  await userEvent.click(screen.getByRole('switch', { name: 'Sort by ID' }))
  expect(
    screen.getByRole('button', { name: 'Reorder channel 2' })
  ).toHaveAccessibleDescription(
    'Sort by priority descending to reorder channels.'
  )
})

it.each([
  { items: [] },
  { items: [{ ...channel('single'), id: 2, priority: 1 }] },
])(
  'does not permit reordering an empty or single-row page',
  async ({ items }) => {
    const post = vi.spyOn(api, 'post')
    await renderPriorityPage({ items })
    expect(
      screen.getByText(
        'At least two channels on this page are needed to reorder.'
      )
    ).toBeInTheDocument()
    for (const handle of screen.queryAllByRole('button', {
      name: /^Reorder channel/,
    })) {
      expect(handle).toHaveAttribute('aria-disabled', 'true')
    }
    expect(post).not.toHaveBeenCalled()
  }
)

it('requires a complete list snapshot before enabling moves', async () => {
  await renderPriorityPage({ snapshot: '' })
  expect(
    screen.getByRole('button', { name: 'Reorder channel 2' })
  ).toHaveAccessibleDescription(
    'Refresh the complete priority list to enable reordering.'
  )
})

function channelRow(id: number) {
  const row = screen
    .getByRole('button', { name: `Reorder channel ${id}` })
    .closest('tr')
  if (!row) throw new Error(`Channel ${id} row not found`)
  return row
}

function priorityCell(id: number) {
  const row = channelRow(id)
  const index = screen
    .getAllByRole('columnheader')
    .findIndex((cell) => cell.textContent === 'Priority')
  return within(row).getAllByRole('cell')[index]
}

it('keeps a pending inline priority edit locked through its debounce, request, and fresh snapshot', async () => {
  let resolve!: (value: unknown) => void
  const put = vi.spyOn(api, 'put').mockImplementation(
    () =>
      new Promise((r) => {
        resolve = r
      }) as never
  )
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: false, code: 'channel_order_failed' },
  })
  const { state } = await renderPriorityPage()
  await userEvent.click(
    within(priorityCell(4)).getByRole('button', { name: 'Increment' })
  )
  const handle = screen.getByRole('button', { name: 'Reorder channel 2' })
  expect(handle).toHaveAccessibleDescription(
    'Wait for pending priority edits to finish before reordering.'
  )
  expect(
    within(priorityCell(4)).getByRole('button', { name: '11' })
  ).toBeInTheDocument()
  await userEvent.click(handle)
  await userEvent.keyboard(' {ArrowUp}{Enter}')
  expect(post).not.toHaveBeenCalled()
  expect(put).toHaveBeenCalledWith(
    '/api/channel/',
    { id: 4, priority: 11 },
    expect.anything()
  )
  state.snapshot = 'after-inline-edit'
  resolve({ data: { success: true } })
  await waitFor(() => expect(handle).toHaveAttribute('aria-disabled', 'false'))
  handle.focus()
  await userEvent.keyboard(' {ArrowUp}{Enter}')
  expect(post).toHaveBeenCalledWith(
    '/api/channel/priority/reorder',
    expect.objectContaining({ expected_snapshot: 'after-inline-edit' }),
    expect.anything()
  )
})

it('preserves the selected channel when its row is moved', async () => {
  vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: false, code: 'channel_order_failed' },
  })
  await renderPriorityPage()
  await userEvent.click(
    screen.getByRole('switch', { name: 'Batch Operations' })
  )
  const handle = screen.getByRole('button', { name: 'Reorder channel 2' })
  const row = channelRow(2)
  await userEvent.click(
    within(row).getByRole('checkbox', { name: 'Select row' })
  )
  handle.focus()
  await userEvent.keyboard(' {ArrowUp}')
  expect(
    within(channelRow(2)).getByRole('checkbox', { name: 'Select row' })
  ).toBeChecked()
  await userEvent.keyboard('{Escape}')
})

it('releases a pointer click without movement so it cannot leave the table locked', async () => {
  const post = vi.spyOn(api, 'post')
  await renderPriorityPage()
  const handle = screen.getByRole('button', { name: 'Reorder channel 2' })
  await userEvent.click(handle)
  expect(handle).toHaveAttribute('aria-pressed', 'false')
  expect(
    screen.getByRole('button', { name: 'Reorder channel 4' })
  ).toHaveAttribute('aria-disabled', 'false')
  expect(post).not.toHaveBeenCalled()
})

it.each([
  ['Priority', 'Asc'],
  ['Name', 'Desc'],
])(
  'disables moves for %s %s and allows an explicit priority-descending view',
  async (column, direction) => {
    await renderPriorityPage()
    await userEvent.click(screen.getByRole('button', { name: column }))
    await userEvent.click(screen.getByRole('menuitem', { name: direction }))
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Reorder channel 2' })
      ).toHaveAccessibleDescription(
        'Sort by priority descending to reorder channels.'
      )
    )
    await userEvent.click(screen.getByRole('button', { name: 'Priority' }))
    await userEvent.click(screen.getByRole('menuitem', { name: 'Desc' }))
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Reorder channel 2' })
      ).toHaveAttribute('aria-disabled', 'false')
    )
  }
)

it('keeps an in-flight tag priority edit locked when returning to the individual-channel view', async () => {
  let resolve!: (value: unknown) => void
  vi.spyOn(api, 'put').mockImplementation(
    () =>
      new Promise((r) => {
        resolve = r
      }) as never
  )
  await renderPriorityPage()
  await userEvent.click(screen.getByRole('switch', { name: 'Tag Mode' }))
  const row = (await screen.findByText('production')).closest('tr')
  if (!row) throw new Error('Tag aggregate row not found')
  const index = screen
    .getAllByRole('columnheader')
    .findIndex((cell) => cell.textContent === 'Priority')
  await userEvent.click(
    within(within(row).getAllByRole('cell')[index]).getByRole('button', {
      name: 'Increment',
    })
  )
  await userEvent.click(screen.getByRole('button', { name: 'Update' }))
  await userEvent.click(screen.getByRole('switch', { name: 'Tag Mode' }))
  const handle = await screen.findByRole('button', {
    name: 'Reorder channel 2',
  })
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Refresh' })).toHaveAttribute(
      'aria-busy',
      'false'
    )
  )
  expect(handle).toHaveAccessibleDescription(
    'Wait for pending priority edits to finish before reordering.'
  )
  resolve({ data: { success: true } })
  await waitFor(() => expect(handle).toHaveAttribute('aria-disabled', 'false'))
})
