import { api } from '@/lib/api'
import type { OnlineUsePageContentResponse } from './types'

export async function getOnlineUsePageContent(): Promise<OnlineUsePageContentResponse> {
  const res = await api.get('/api/online_use_page_content')
  return res.data
}
