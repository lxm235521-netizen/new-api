import { useTranslation } from 'react-i18next'
import { Markdown } from '@/components/ui/markdown'
import { PublicLayout } from '@/components/layout'
import { usePluginPageContent } from './hooks'

export function Plugins() {
  const { t } = useTranslation()
  const { content, isLoaded, isUrl, isHtml } = usePluginPageContent()

  if (!isLoaded) {
    return (
      <PublicLayout showMainContainer={false}>
        <main className='flex min-h-screen items-center justify-center'>
          <div className='text-muted-foreground'>{t('Loading...')}</div>
        </main>
      </PublicLayout>
    )
  }

  if (!content) {
    return (
      <PublicLayout showMainContainer={false}>
        <main className='flex min-h-screen items-center justify-center'>
          <div className='text-muted-foreground'>{t('No content')}</div>
        </main>
      </PublicLayout>
    )
  }

  return (
    <PublicLayout showMainContainer={false}>
      <main className='overflow-x-hidden'>
        {isUrl ? (
          <iframe
            src={content}
            className='h-screen w-full border-none'
            title={t('Plugin Download')}
          />
        ) : isHtml ? (
          <iframe
            srcDoc={content}
            className='h-screen w-full border-none'
            title={t('Plugin Download')}
            sandbox='allow-scripts allow-same-origin'
          />
        ) : (
          <div className='container mx-auto py-8'>
            <Markdown className='custom-home-content'>{content}</Markdown>
          </div>
        )}
      </main>
    </PublicLayout>
  )
}
