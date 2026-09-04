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
import { useMutation, useQuery } from '@tanstack/react-query'
import { Banknote, ChevronLeft, ChevronRight } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table'
import {
  sideDrawerContentClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatQuota, formatTimestamp } from '@/lib/format'

import {
  clearInvitationRewardQuota,
  getInvitationRewardRecords,
  getInvitedUsers,
} from '../../api'
import { USER_STATUS } from '../../constants'
import type { InvitationRewardRecord, User } from '../../types'

const PAGE_SIZE = 20

interface InvitationRewardsSheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  user: User | null
  onSuccess?: () => void
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

function RewardTypeBadge(props: { record: InvitationRewardRecord }) {
  const { t } = useTranslation()
  if (props.record.type === 'signup') {
    return (
      <StatusBadge
        label={t('Signup Reward')}
        variant='neutral'
        copyable={false}
      />
    )
  }
  return (
    <StatusBadge
      label={t('Recharge Commission')}
      variant='success'
      copyable={false}
    />
  )
}

export function InvitationRewardsSheet(props: InvitationRewardsSheetProps) {
  const { t } = useTranslation()
  const [invitedPage, setInvitedPage] = useState(1)
  const [rewardPage, setRewardPage] = useState(1)
  const [clearConfirmOpen, setClearConfirmOpen] = useState(false)
  const userId = props.user?.id ?? 0

  const invitedQuery = useQuery({
    queryKey: ['invited-users', userId, invitedPage],
    queryFn: () => getInvitedUsers(userId, invitedPage, PAGE_SIZE),
    enabled: props.open && userId > 0,
  })
  const rewardsQuery = useQuery({
    queryKey: ['invitation-rewards', userId, rewardPage],
    queryFn: () => getInvitationRewardRecords(userId, rewardPage, PAGE_SIZE),
    enabled: props.open && userId > 0,
  })
  const clearMutation = useMutation({
    mutationFn: () => clearInvitationRewardQuota(userId),
    onSuccess: (result) => {
      if (!result.success) {
        toast.error(result.message || t('Operation failed'))
        return
      }
      toast.success(
        t('Cleared {{quota}} in pending invitation rewards', {
          quota: formatQuota(result.data?.cleared_quota ?? 0),
        })
      )
      setClearConfirmOpen(false)
      props.onSuccess?.()
    },
    onError: () => toast.error(t('Operation failed')),
  })

  const invitedData = invitedQuery.data?.data
  const rewardData = rewardsQuery.data?.data
  const pendingReward = props.user?.aff_quota ?? 0

  return (
    <>
      <Sheet open={props.open} onOpenChange={props.onOpenChange}>
        <SheetContent className={sideDrawerContentClassName('sm:max-w-3xl')}>
          <SheetHeader className={sideDrawerHeaderClassName()}>
            <SheetTitle>{t('Invitation Reward Management')}</SheetTitle>
            <SheetDescription>
              {props.user?.username || '-'} (ID: {userId || '-'})
            </SheetDescription>
          </SheetHeader>

          <div className={sideDrawerFormClassName()}>
            <div className='grid gap-3 sm:grid-cols-3'>
              <Card>
                <CardHeader className='gap-1 py-4'>
                  <CardDescription>{t('Pending Rewards')}</CardDescription>
                  <CardTitle className='text-xl'>
                    {formatQuota(pendingReward)}
                  </CardTitle>
                </CardHeader>
              </Card>
              <Card>
                <CardHeader className='gap-1 py-4'>
                  <CardDescription>{t('Total Earned')}</CardDescription>
                  <CardTitle className='text-xl'>
                    {formatQuota(props.user?.aff_history_quota ?? 0)}
                  </CardTitle>
                </CardHeader>
              </Card>
              <Card>
                <CardHeader className='gap-1 py-4'>
                  <CardDescription>{t('Invited Users')}</CardDescription>
                  <CardTitle className='text-xl'>
                    {props.user?.aff_count ?? 0}
                  </CardTitle>
                </CardHeader>
              </Card>
            </div>

            <div className='flex justify-end'>
              <Button
                variant='outline'
                disabled={pendingReward <= 0}
                onClick={() => setClearConfirmOpen(true)}
              >
                <Banknote data-icon='inline-start' />
                {t('Mark Paid and Clear')}
              </Button>
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
                  getRowKey={(user) => user.id}
                  emptyContent={
                    invitedQuery.isLoading
                      ? t('Loading...')
                      : t('No invited users')
                  }
                  columns={[
                    { id: 'id', header: t('ID'), cell: (user) => user.id },
                    {
                      id: 'user',
                      header: t('User'),
                      cell: (user) => user.display_name || user.username,
                    },
                    {
                      id: 'status',
                      header: t('Status'),
                      cell: (user) => (
                        <StatusBadge
                          label={
                            user.status === USER_STATUS.ENABLED
                              ? t('Enabled')
                              : t('Disabled')
                          }
                          variant={
                            user.status === USER_STATUS.ENABLED
                              ? 'success'
                              : 'neutral'
                          }
                          copyable={false}
                        />
                      ),
                    },
                    {
                      id: 'created-at',
                      header: t('Registered At'),
                      cell: (user) => formatTimestamp(user.created_at),
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
                  getRowKey={(record) => record.id}
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
                      cell: (record) =>
                        record.invitee_username || `#${record.invitee_id}`,
                    },
                    {
                      id: 'basis',
                      header: t('Calculation'),
                      cell: (record) =>
                        record.type === 'recharge'
                          ? `${formatQuota(record.base_quota)} × ${record.rate}%`
                          : '-',
                    },
                    {
                      id: 'reward',
                      header: t('Reward'),
                      cell: (record) => formatQuota(record.reward_quota),
                    },
                    {
                      id: 'created-at',
                      header: t('Created At'),
                      cell: (record) => formatTimestamp(record.created_at),
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

      <ConfirmDialog
        open={clearConfirmOpen}
        onOpenChange={setClearConfirmOpen}
        title={t('Mark Invitation Rewards as Paid')}
        desc={t(
          'Confirm only after the user has been paid manually. This clears the pending reward without adding it to the user balance.'
        )}
        confirmText={t('Mark Paid and Clear')}
        destructive
        isLoading={clearMutation.isPending}
        handleConfirm={() => clearMutation.mutate()}
      />
    </>
  )
}
