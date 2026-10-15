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
import { TruncatedText } from '@/components/truncated-text'

import type { Channel } from '../types'

interface ChannelNameCellProps {
  channel: Pick<Channel, 'name' | 'base_url'>
  sensitiveVisible: boolean
  defaultBaseURL?: string
}

export function ChannelNameCell(props: ChannelNameCellProps) {
  const name = (
    <TruncatedText
      text={props.sensitiveVisible ? props.channel.name : '••••'}
      className='font-medium'
      maxWidth='max-w-full'
    />
  )
  if (!props.sensitiveVisible) return name

  const baseURL = props.channel.base_url || props.defaultBaseURL
  if (!baseURL || !/^https?:\/\//i.test(baseURL) || /[{}\s]/.test(baseURL)) {
    return name
  }

  try {
    const url = new URL(baseURL)
    if (!url.hostname || url.username || url.password) return name
  } catch {
    return name
  }

  return (
    <a
      href={baseURL}
      rel='noreferrer'
      referrerPolicy='no-referrer'
      className='min-w-0 rounded-sm hover:underline focus-visible:underline'
    >
      {name}
    </a>
  )
}
