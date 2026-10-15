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

async function renderPriorityPage() {
  const items = [
    { ...channel('D'), id: 4, priority: 10 },
    { ...channel('E'), id: 3, priority: 10 },
    { ...channel('F'), id: 2, priority: 5, status: 2 },
  ]
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/channel') {
      return {
        data: {
          success: true,
          data: {
            items,
            total: 6,
            page: 2,
            page_size: 3,
            order_snapshot: 'baseline',
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
        <ChannelsTable />
      </ChannelsProvider>
    ),
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([channelsRoute])]),
    history: createMemoryHistory({
      initialEntries: ['/channels/?page=2&pageSize=3'],
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
  await screen.findByText('D')
  await userEvent.click(screen.getByRole('button', { name: 'Table view' }))
  await screen.findByRole('button', { name: 'Reorder channel 2' })
  return { get, items }
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
