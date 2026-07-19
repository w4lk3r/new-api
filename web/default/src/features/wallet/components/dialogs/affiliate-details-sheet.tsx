/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import {
  sideDrawerContentClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatQuota } from '@/lib/format'

import { getSelfInvitationRewards, getSelfInvitedUsers } from '../../api'
import type { AffiliateRewardRecord, UserWalletData } from '../../types'

const PAGE_SIZE = 20

interface AffiliateDetailsSheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  user: UserWalletData | null
}

interface TablePagerProps {
  page: number
  total: number
  onPageChange: (page: number) => void
}

function TablePager(props: TablePagerProps) {
  const { t } = useTranslation()
  const totalPages = Math.max(1, Math.ceil(props.total / PAGE_SIZE))
  if (props.total <= PAGE_SIZE) return null

  return (
    <div className='flex items-center justify-end gap-2 pt-3'>
      <span className='text-muted-foreground text-xs'>
        {t('Page {{page}} of {{total}}', {
          page: props.page,
          total: totalPages,
        })}
      </span>
      <Button
        variant='outline'
        size='icon-sm'
        disabled={props.page <= 1}
        onClick={() => props.onPageChange(props.page - 1)}
        aria-label={t('Previous page')}
      >
        <ChevronLeft />
      </Button>
      <Button
        variant='outline'
        size='icon-sm'
        disabled={props.page >= totalPages}
        onClick={() => props.onPageChange(props.page + 1)}
        aria-label={t('Next page')}
      >
        <ChevronRight />
      </Button>
    </div>
  )
}

function RewardTypeBadge(props: { record: AffiliateRewardRecord }) {
  const { t } = useTranslation()
  return (
    <StatusBadge
      label={
        props.record.type === 'signup'
          ? t('Signup Reward')
          : t('Recharge Commission')
      }
      variant={props.record.type === 'signup' ? 'neutral' : 'success'}
      copyable={false}
    />
  )
}

export function AffiliateDetailsSheet(props: AffiliateDetailsSheetProps) {
  const { t } = useTranslation()
  const [invitedPage, setInvitedPage] = useState(1)
  const [rewardPage, setRewardPage] = useState(1)

  const invitedQuery = useQuery({
    queryKey: ['self-invited-users', invitedPage],
    queryFn: () => getSelfInvitedUsers(invitedPage, PAGE_SIZE),
    enabled: props.open,
  })
  const rewardsQuery = useQuery({
    queryKey: ['self-invitation-rewards', rewardPage],
    queryFn: () => getSelfInvitationRewards(rewardPage, PAGE_SIZE),
    enabled: props.open,
  })

  const invitedData = invitedQuery.data?.data
  const rewardData = rewardsQuery.data?.data

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-3xl')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>{t('Referral Details')}</SheetTitle>
          <SheetDescription>
            {t('Exact recharge amounts and times are hidden for privacy.')}
          </SheetDescription>
        </SheetHeader>

        <div className={sideDrawerFormClassName()}>
          <div className='grid gap-3 sm:grid-cols-3'>
            <div className='bg-muted/40 rounded-lg border p-3'>
              <div className='text-muted-foreground text-xs'>
                {t('Pending Rewards')}
              </div>
              <div className='mt-1 text-lg font-semibold tabular-nums'>
                {formatQuota(props.user?.aff_quota ?? 0)}
              </div>
            </div>
            <div className='bg-muted/40 rounded-lg border p-3'>
              <div className='text-muted-foreground text-xs'>
                {t('Total Earned')}
              </div>
              <div className='mt-1 text-lg font-semibold tabular-nums'>
                {formatQuota(props.user?.aff_history_quota ?? 0)}
              </div>
            </div>
            <div className='bg-muted/40 rounded-lg border p-3'>
              <div className='text-muted-foreground text-xs'>
                {t('Invites')}
              </div>
              <div className='mt-1 text-lg font-semibold tabular-nums'>
                {props.user?.aff_count ?? 0}
              </div>
            </div>
          </div>

          <Tabs defaultValue='invited-users'>
            <TabsList>
              <TabsTrigger value='invited-users'>
                {t('Invited Users')}
              </TabsTrigger>
              <TabsTrigger value='reward-history'>
                {t('Reward History')}
              </TabsTrigger>
            </TabsList>

            <TabsContent value='invited-users' className='pt-3'>
              <StaticDataTable
                data={invitedData?.items ?? []}
                getRowKey={(user, index) => `${user.invitee}-${index}`}
                emptyContent={
                  invitedQuery.isLoading
                    ? t('Loading...')
                    : t('No invited users')
                }
                columns={[
                  {
                    id: 'user',
                    header: t('User'),
                    cell: (user) => user.invitee,
                  },
                  {
                    id: 'registered-date',
                    header: t('Registered At'),
                    cell: (user) => user.registered_date,
                  },
                ]}
              />
              <TablePager
                page={invitedPage}
                total={invitedData?.total ?? 0}
                onPageChange={setInvitedPage}
              />
            </TabsContent>

            <TabsContent value='reward-history' className='pt-3'>
              <StaticDataTable
                data={rewardData?.items ?? []}
                getRowKey={(record, index) =>
                  `${record.type}-${record.reward_date}-${index}`
                }
                emptyContent={
                  rewardsQuery.isLoading
                    ? t('Loading...')
                    : t('No reward records')
                }
                columns={[
                  {
                    id: 'type',
                    header: t('Type'),
                    cell: (record) => <RewardTypeBadge record={record} />,
                  },
                  {
                    id: 'invitee',
                    header: t('Invited User'),
                    cell: (record) => record.invitee,
                  },
                  {
                    id: 'reward',
                    header: t('Reward'),
                    cell: (record) => formatQuota(record.reward_quota),
                  },
                  {
                    id: 'reward-date',
                    header: t('Reward Date'),
                    cell: (record) => record.reward_date,
                  },
                ]}
              />
              <TablePager
                page={rewardPage}
                total={rewardData?.total ?? 0}
                onPageChange={setRewardPage}
              />
            </TabsContent>
          </Tabs>
        </div>
      </SheetContent>
    </Sheet>
  )
}
