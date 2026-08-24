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

const OPTION_KEY = 'task_per_call_billing_setting.model_names'

const schema = z.object({
  'task_per_call_billing_setting.model_names': z.string(),
})

type FormValues = z.infer<typeof schema>

function parseModelNames(value: string): string[] {
  try {
    const parsed = JSON.parse(value) as unknown
    if (Array.isArray(parsed)) {
      return parsed
        .filter((item): item is string => typeof item === 'string')
        .map((item) => item.trim())
        .filter(Boolean)
    }
  } catch {
    return value
      .split(/[\n,]/)
      .map((item) => item.trim())
      .filter(Boolean)
  }

  return value
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean)
}

function modelNamesToText(value: string): string {
  return parseModelNames(value).join('\n')
}

export function TaskPerCallBillingSection({
  defaultValue,
}: {
  defaultValue: string
}) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const formDefaults = useMemo<FormValues>(
    () => ({ [OPTION_KEY]: modelNamesToText(defaultValue) }),
    [defaultValue]
  )
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: formDefaults,
  })

  useEffect(() => {
    form.reset(formDefaults)
  }, [formDefaults, form])

  const handleSubmit = async (values: FormValues) => {
    await updateOption.mutateAsync({
      key: OPTION_KEY,
      value: JSON.stringify(parseModelNames(values[OPTION_KEY])),
    })
  }

  return (
    <SettingsSection title={t('Task Per-Call Billing')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(handleSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(handleSubmit)}
            isSaving={updateOption.isPending}
            saveLabel={t('Save')}
          />
          <FormField
            control={form.control}
            name={OPTION_KEY}
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Model Names')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={8}
                    placeholder={t('Enter one model name per line')}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Listed task models are charged once per request, without duration or size multipliers. Changes apply to new tasks immediately.'
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
