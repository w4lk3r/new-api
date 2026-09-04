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
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import type { User } from '../../../types'
import { InvitationRewardsSheet } from '../invitation-rewards-sheet'

const originalAdapter = api.defaults.adapter
let client: QueryClient
const user: User = {
  id: 7,
  username: 'inviter',
  display_name: 'Inviter',
  quota: 100,
  used_quota: 0,
  request_count: 0,
  group: 'default',
  status: 1,
  role: 1,
  aff_quota: 50,
}

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
  )
  Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  api.defaults.adapter = originalAdapter
  vi.unstubAllGlobals()
  Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
})

describe('invitation administration', () => {
  test('pay-and-clear requires confirmation and calls only the selected user endpoint', async () => {
    const posts: string[] = []
    const success = vi.fn()
    api.defaults.adapter = async (config) => {
      if (config.method === 'post') posts.push(config.url ?? '')
      return {
        data: {
          success: true,
          data:
            config.method === 'post'
              ? { cleared_quota: 50 }
              : { items: [], total: 0 },
        },
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      }
    }
    render(
      <QueryClientProvider client={client}>
        <InvitationRewardsSheet
          open
          onOpenChange={() => undefined}
          user={user}
          onSuccess={success}
        />
      </QueryClientProvider>
    )
    expect(await screen.findByText('No invited users')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Mark Paid and Clear' }))
    const confirmation = await screen.findByRole('alertdialog')
    expect(
      within(confirmation).getByText(
        'Confirm only after the user has been paid manually. This clears the pending reward without adding it to the user balance.'
      )
    ).toBeVisible()
    expect(posts).toEqual([])
    fireEvent.click(
      within(confirmation).getByRole('button', { name: 'Mark Paid and Clear' })
    )
    await waitFor(() => expect(success).toHaveBeenCalledOnce())
    expect(posts).toEqual(['/api/user/7/invitation-rewards/clear'])
  })

  test('zero pending rewards disables pay-and-clear', () => {
    client.setQueryData(['invited-users', 7, 1], {
      success: true,
      data: { items: [], total: 0 },
    })
    client.setQueryData(['invitation-rewards', 7, 1], {
      success: true,
      data: { items: [], total: 0 },
    })
    api.defaults.adapter = async (config) => ({
      data: { success: true, data: { items: [], total: 0 } },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    })
    render(
      <QueryClientProvider client={client}>
        <InvitationRewardsSheet
          open
          onOpenChange={() => undefined}
          user={{ ...user, aff_quota: 0 }}
        />
      </QueryClientProvider>
    )
    expect(
      screen.getByRole('button', { name: 'Mark Paid and Clear' })
    ).toBeDisabled()
  })

  test('failed clear keeps confirmation open and does not report success', async () => {
    const success = vi.fn()
    api.defaults.adapter = async (config) => ({
      data:
        config.method === 'post'
          ? { success: false, message: 'Payment was not recorded' }
          : { success: true, data: { items: [], total: 0 } },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    })
    render(
      <QueryClientProvider client={client}>
        <InvitationRewardsSheet
          open
          onOpenChange={() => undefined}
          user={user}
          onSuccess={success}
        />
      </QueryClientProvider>
    )
    fireEvent.click(screen.getByRole('button', { name: 'Mark Paid and Clear' }))
    const confirmation = await screen.findByRole('alertdialog')
    fireEvent.click(
      within(confirmation).getByRole('button', { name: 'Mark Paid and Clear' })
    )
    await waitFor(() =>
      expect(
        within(confirmation).getByRole('button', {
          name: 'Mark Paid and Clear',
        })
      ).toBeEnabled()
    )
    expect(confirmation).toBeVisible()
    expect(success).not.toHaveBeenCalled()
  })
})
