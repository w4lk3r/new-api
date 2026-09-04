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
import { generateKeyPairSync, privateDecrypt } from 'node:crypto'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { AxiosError, type InternalAxiosRequestConfig } from 'axios'
import i18next from 'i18next'
import { toast } from 'sonner'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { clearPasswordEncryptionCache } from '../lib/password-encryption'
import { SignIn } from '../sign-in'
import { SignUp } from '../sign-up'
import type { SystemStatus } from '../types'

const originalAdapter = api.defaults.adapter
let queryClient: QueryClient

async function showAuth(status: SystemStatus, path = '/sign-in') {
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  queryClient.setQueryData(['status'], status)
  queryClient.setQueryData(['notice'], {
    success: true,
    data: 'Scheduled maintenance notice',
  })
  const root = createRootRoute({ component: Outlet })
  const auth = createRoute({
    getParentRoute: () => root,
    id: '(auth)',
    component: Outlet,
  })
  const signIn = createRoute({
    getParentRoute: () => auth,
    path: 'sign-in',
    component: SignIn,
  })
  const signUp = createRoute({
    getParentRoute: () => auth,
    path: 'sign-up',
    component: SignUp,
  })
  const otp = createRoute({
    getParentRoute: () => root,
    path: 'otp',
    component: () => <p>Second factor required</p>,
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([signIn, signUp]), otp]),
    history: createMemoryHistory({ initialEntries: [path] }),
  })
  await act(async () => {
    render(
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    )
  })
  return router
}

beforeEach(() => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  localStorage.clear()
  window.history.replaceState({}, '', '/sign-in')
  clearPasswordEncryptionCache()
  useAuthStore.getState().auth.reset()
  api.defaults.adapter = async (config) => {
    throw new Error(`Unexpected request ${config.url}`)
  }
})

afterEach(async () => {
  cleanup()
  queryClient?.clear()
  api.defaults.adapter = originalAdapter
  clearPasswordEncryptionCache()
  useAuthStore.getState().auth.reset()
  localStorage.clear()
  await i18next.changeLanguage('en')
})

describe('private login behavior on upstream authentication', () => {
  test('disabled password login hides the form and password registration link while showing the notice', async () => {
    await showAuth({
      password_login_enabled: false,
      password_register_enabled: false,
    })
    expect(
      await screen.findByText('Scheduled maintenance notice')
    ).toBeVisible()
    expect(
      screen.queryByRole('textbox', { name: 'Username or Email' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Sign up' })
    ).not.toBeInTheDocument()
  })

  test('admin entry retains encryption and the upstream 2FA flow token', async () => {
    window.history.replaceState({}, '', '/sign-in?admin=true')
    const keys = generateKeyPairSync('rsa', { modulusLength: 2048 })
    let payload: Record<string, string> | undefined
    api.defaults.adapter = async (config) => {
      let data: unknown
      if (config.url === '/api/user/login/encryption-key') {
        data = {
          success: true,
          data: {
            kid: 'key-1',
            public_key: keys.publicKey.export({ type: 'spki', format: 'pem' }),
          },
        }
      } else if (config.url === '/api/user/login?turnstile=') {
        payload = JSON.parse(config.data as string)
        data = {
          success: true,
          data: { require_2fa: true, flow_token: 'second-factor-flow' },
        }
      } else throw new Error(`Unexpected request ${config.url}`)
      return { data, status: 200, statusText: 'OK', headers: {}, config }
    }
    await showAuth({
      password_login_enabled: false,
      password_login_encryption_enabled: true,
    })
    fireEvent.change(
      await screen.findByRole('textbox', { name: 'Username or Email' }),
      { target: { value: 'admin' } }
    )
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: 'secret-passphrase' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }))
    expect(await screen.findByText('Second factor required')).toBeVisible()
    expect(payload).not.toHaveProperty('password')
    expect(payload?.encryption_key_id).toBe('key-1')
    expect(
      privateDecrypt(
        { key: keys.privateKey, oaepHash: 'sha256' },
        Buffer.from(payload?.password_encrypted ?? '', 'base64')
      ).toString()
    ).toBe('secret-passphrase')
    expect(useAuthStore.getState().auth.pending2FAFlowToken).toBe(
      'second-factor-flow'
    )
  })

  test.each([
    { register_enabled: false },
    { password_register_enabled: false },
  ])('closed registration redirects to login for %j', async (status) => {
    const router = await showAuth(status, '/sign-up')
    await waitFor(() => expect(router.state.location.pathname).toBe('/sign-in'))
    expect(
      screen.queryByRole('link', { name: 'Sign up' })
    ).not.toBeInTheDocument()
  })

  test('default OIDC uses the private Chinese label and preserves the official flow request', async () => {
    i18next.addResourceBundle('zh', 'translation', zh.translation)
    await i18next.changeLanguage('zh')
    const opened = vi.spyOn(window, 'open').mockReturnValue(null)
    const requests: { url: string | undefined; data: unknown }[] = []
    api.defaults.adapter = async (config) => {
      requests.push({
        url: config.url,
        data: config.data ? JSON.parse(config.data as string) : undefined,
      })
      return {
        data: { success: true, data: { flow_token: 'oidc-flow' } },
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      }
    }
    await showAuth({
      password_login_enabled: false,
      oidc_enabled: true,
      oidc_client_id: 'client',
      oidc_authorization_endpoint: 'https://identity.example/authorize',
    })
    fireEvent.click(await screen.findByRole('button', { name: '立即登录' }))
    await waitFor(() => expect(opened).toHaveBeenCalled())
    expect(requests.map((request) => request.url)).toEqual([
      '/api/user/auth/logout',
      '/api/oauth/state',
    ])
    expect(requests[1].data).toEqual({ provider: 'oidc', intent: 'login' })
    const url = new URL(opened.mock.calls[0][0] as string)
    expect(url.searchParams.get('state')).toBe('oidc-flow')
  })

  test.each(['/api/user/auth/logout', '/api/oauth/state'])(
    '429 at %s displays one error and re-enables OIDC login',
    async (failureURL) => {
      const errorToast = vi.spyOn(toast, 'error')
      api.defaults.adapter = async (config: InternalAxiosRequestConfig) => {
        const response = {
          data: { message: 'Too many requests' },
          status: 429,
          statusText: 'Too Many Requests',
          headers: {},
          config,
        }
        if (config.url === failureURL) {
          throw new AxiosError(
            'Too many requests',
            'ERR_BAD_REQUEST',
            config,
            undefined,
            response
          )
        }
        return { ...response, data: { success: true }, status: 200 }
      }
      await showAuth({
        oidc_enabled: true,
        oidc_client_id: 'client',
        oidc_authorization_endpoint: 'https://identity.example/authorize',
      })
      fireEvent.click(
        await screen.findByRole('button', { name: 'Continue with OIDC' })
      )
      await waitFor(() =>
        expect(errorToast.mock.calls).toEqual([
          [
            failureURL === '/api/user/auth/logout'
              ? 'Failed to start OIDC login'
              : 'Too many requests',
          ],
        ])
      )
      await waitFor(() =>
        expect(
          screen.getByRole('button', { name: 'Continue with OIDC' })
        ).toBeEnabled()
      )
    }
  )
})
