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
import { getCoreRowModel, useReactTable } from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'

import { DataTableRow } from '../data-table-row'
import { DataTableView } from '../data-table-view'

const channels = [{ id: 'disabled', name: 'Disabled channel' }]
const columns = [{ accessorKey: 'name', header: 'Name' }]

function Fixture(props: { customRow: boolean }) {
  const table = useReactTable({
    data: channels,
    columns,
    getCoreRowModel: getCoreRowModel(),
    getRowId: (row) => row.id,
    initialState: { rowSelection: { disabled: true } },
  })
  return (
    <DataTableView
      table={table}
      tableBodyRowClassName='min-h-12'
      getRowClassName={() => '[background-color:var(--table-disabled)]'}
      renderRow={
        props.customRow
          ? (row, helpers) => (
              <DataTableRow
                key={row.id}
                row={row}
                className={helpers.className}
                getColumnClassName={helpers.getCellClassName}
                render={(rowProps) => <tr {...rowProps} />}
              />
            )
          : undefined
      }
    />
  )
}

it.each([false, true])(
  'preserves disabled and selected row styling when a custom renderer is %s',
  (customRow) => {
    render(<Fixture customRow={customRow} />)
    const row = screen.getByRole('row', { name: 'Disabled channel' })
    expect(row).toHaveClass(
      'min-h-12',
      '[background-color:var(--table-disabled)]'
    )
    expect(row).toHaveAttribute('data-state', 'selected')
    expect(screen.getByRole('cell', { name: 'Disabled channel' })).toBeVisible()
  }
)
