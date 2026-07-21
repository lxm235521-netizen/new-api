import { useTranslation } from 'react-i18next'
import { Markdown } from '@/components/ui/markdown'
import { PublicLayout } from '@/components/layout'
import { useOnlineUsePageContent } from './hooks'

export function OnlineUse() {
  const { t } = useTranslation()
  const { content, isLoaded, isUrl, isHtml } = useOnlineUsePageContent()

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
            title={t('Online Use')}
            sandbox='allow-scripts allow-same-origin allow-downloads allow-popups'
          />
        ) : isHtml ? (
          <iframe
            srcDoc={content}
            className='h-screen w-full border-none'
            title={t('Online Use')}
            sandbox='allow-scripts allow-same-origin allow-downloads allow-popups'
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
