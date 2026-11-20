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
import { useEffect, useMemo } from 'react'
import * as z from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Textarea } from '@/components/ui/textarea'
import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const OPTION_KEY = 'billing_setting.resolution_price'

const EXAMPLE = `{
  "veo-3.1-generate-preview": {
    "480p": 0.2,
    "720p": 0.3,
    "768p": 0.35,
    "1080p": 0.5,
    "4k": 1.2
  }
}`

const schema = z.object({
  resolutionPrice: z.string(),
})

type FormValues = z.infer<typeof schema>

function formatJson(value: string): string {
  try {
    return JSON.stringify(JSON.parse(value || '{}'), null, 2)
  } catch {
    return value || '{}'
  }
}

export function ResolutionPricingCard({ defaultValue }: { defaultValue: string }) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const formDefaults = useMemo<FormValues>(
    () => ({ resolutionPrice: formatJson(defaultValue) }),
    [defaultValue]
  )

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: formDefaults,
  })

  useEffect(() => {
    form.reset(formDefaults)
  }, [formDefaults, form])

  const onSubmit = async (values: FormValues) => {
    try {
      JSON.parse(values.resolutionPrice)
    } catch {
      form.setError('resolutionPrice', {
        message: t('Invalid JSON'),
      })
      return
    }
    await updateOption.mutateAsync({
      key: OPTION_KEY,
      value: values.resolutionPrice,
    })
  }

  return (
    <SettingsSection title={t('Resolution Pricing')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
          />
          <FormField
            control={form.control}
            name='resolutionPrice'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Resolution Pricing')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={12}
                    placeholder={EXAMPLE}
                    className='font-mono text-xs'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    "JSON map of model to resolution prices. The price unit follows the model's per-call/per-second setting. The request's resolution field is read first."
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
