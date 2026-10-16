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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'

import { ChannelNameCell } from '../channel-name-cell'

test.each(['https://upstream.example/v1?region=cn', 'http://localhost:11434'])(
  'a channel with %s opens its configured address in a new tab without a referrer',
  async (baseURL) => {
    render(
      <ChannelNameCell
        channel={{ name: '123', base_url: baseURL }}
        sensitiveVisible
        defaultBaseURL='https://default.example'
      />
    )

    const link = screen.getByRole('link', { name: '123' })
    expect(link).toHaveAttribute('href', baseURL)
    expect(link).toHaveAttribute('rel', 'noreferrer')
    expect(link).toHaveAttribute('referrerpolicy', 'no-referrer')
    expect(link).toHaveAttribute('target', '_blank')
    await userEvent.setup().tab()
    expect(link).toHaveFocus()
  }
)

test.each(['', null, undefined])(
  'an unset configured address (%s) uses the channel type default',
  (baseURL) => {
    render(
      <ChannelNameCell
        channel={{ name: 'Default upstream', base_url: baseURL }}
        sensitiveVisible
        defaultBaseURL='https://default.example'
      />
    )

    expect(
      screen.getByRole('link', { name: 'Default upstream' })
    ).toHaveAttribute('href', 'https://default.example')
  }
)

test.each([
  'javascript:alert(1)',
  'data:text/html,test',
  '//upstream.example',
  'glm-coding-plan',
  'https://',
  'https://upstream.example/{model}',
  'https://upstream.example/path with spaces',
  'https://username:password@upstream.example',
])(
  'a configured non-navigable address (%s) stays plain text even when a default exists',
  (baseURL) => {
    render(
      <ChannelNameCell
        channel={{ name: 'Special upstream', base_url: baseURL }}
        sensitiveVisible
        defaultBaseURL='https://default.example'
      />
    )

    expect(screen.queryByRole('link')).not.toBeInTheDocument()
    expect(screen.getByText('Special upstream')).toBeVisible()
  }
)

test('a channel without a configured or available default address stays plain text', () => {
  render(
    <ChannelNameCell
      channel={{ name: 'No address', base_url: '' }}
      sensitiveVisible
    />
  )

  expect(screen.queryByRole('link')).not.toBeInTheDocument()
  expect(screen.getByText('No address')).toBeVisible()
})

test('hiding sensitive data removes the channel name and its upstream link', () => {
  const props = {
    channel: { name: 'Private upstream', base_url: 'https://private.example' },
    defaultBaseURL: 'https://default.example',
  }
  const view = render(<ChannelNameCell {...props} sensitiveVisible />)
  expect(screen.getByRole('link', { name: 'Private upstream' })).toBeVisible()

  view.rerender(<ChannelNameCell {...props} sensitiveVisible={false} />)

  expect(screen.queryByRole('link')).not.toBeInTheDocument()
  expect(screen.queryByText('Private upstream')).not.toBeInTheDocument()
  expect(screen.getByText('••••')).toBeVisible()
})
