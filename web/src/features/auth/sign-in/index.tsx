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
import { useQuery } from '@tanstack/react-query'
import { Link, useSearch } from '@tanstack/react-router'
import { Volume2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { RichContent } from '@/components/rich-content'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { useStatus } from '@/hooks/use-status'
import { getNotice } from '@/lib/api'

import { AuthLayout } from '../auth-layout'
import { TermsFooter } from '../components/terms-footer'
import { UserAuthForm } from './components/user-auth-form'

export function SignIn() {
  const { t } = useTranslation()
  const { redirect } = useSearch({ from: '/(auth)/sign-in' })
  const { status } = useStatus()

  const { data: noticeResponse } = useQuery({
    queryKey: ['notice'],
    queryFn: getNotice,
    staleTime: 1000 * 60 * 5, // 5 minutes
  })

  const notice = noticeResponse?.success
    ? (noticeResponse.data || '').trim()
    : ''

  return (
    <AuthLayout>
      <div className='w-full space-y-8'>
        <div className='space-y-2'>
          <h2 className='text-center text-2xl font-semibold tracking-tight sm:text-left'>
            {t('Sign in')}
          </h2>
          {!status?.self_use_mode_enabled &&
            status?.register_enabled !== false &&
            status?.password_register_enabled !== false && (
              <p className='text-muted-foreground text-left text-sm sm:text-base'>
                {t("Don't have an account?")}{' '}
                <Link
                  to='/sign-up'
                  className='hover:text-primary font-medium underline underline-offset-4'
                >
                  {t('Sign up')}
                </Link>
                .
              </p>
            )}
        </div>

        {notice && (
          <Alert className='border-amber-200/50 bg-amber-50/20 dark:border-amber-900/30 dark:bg-amber-950/10 [&>svg]:text-amber-600 [&>svg]:dark:text-amber-400'>
            <Volume2 className='h-4 w-4' />
            <AlertDescription className='text-xs leading-relaxed break-words text-amber-800 dark:text-amber-300'>
              <RichContent
                breaks
                content={notice}
                className='text-xs text-amber-800 dark:text-amber-300'
              />
            </AlertDescription>
          </Alert>
        )}

        <UserAuthForm redirectTo={redirect} />

        <TermsFooter
          variant='sign-in'
          status={status}
          className='text-center'
        />
      </div>
    </AuthLayout>
  )
}
