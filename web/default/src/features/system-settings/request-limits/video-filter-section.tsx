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

const schema = z.object({
  'video_filter_setting.hidden_fields_models': z.string(),
})

type FormValues = z.infer<typeof schema>

type VideoFilterSectionProps = {
  defaultValues: {
    'video_filter_setting.hidden_fields_models': string[]
  }
}

function modelsToText(models: string[]): string {
  return models.join('\n')
}

function textToModels(text: string): string[] {
  return text
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)
}

export function VideoFilterSection({ defaultValues }: VideoFilterSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const formDefaults = useMemo<FormValues>(
    () => ({
      'video_filter_setting.hidden_fields_models': modelsToText(
        defaultValues['video_filter_setting.hidden_fields_models']
      ),
    }),
    [defaultValues]
  )

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: formDefaults,
  })

  useEffect(() => {
    form.reset(formDefaults)
  }, [formDefaults, form])

  const onSubmit = async (values: FormValues) => {
    const key = 'video_filter_setting.hidden_fields_models'
    const newModels = textToModels(values[key])
    const oldModels = defaultValues[key]
    if (newModels.join(',') === oldModels.join(',')) return

    await updateOption.mutateAsync({
      key,
      value: JSON.stringify(newModels),
    })
  }

  return (
    <SettingsSection title={t('Response Field Filtering')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel={t('Save')}
          />
          <FormField
            control={form.control}
            name='video_filter_setting.hidden_fields_models'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Models')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={8}
                    placeholder={t('Enter one model name per line')}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'video_url and download_url fields in task responses will be hidden for the listed models. This does not affect video playback.'
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
