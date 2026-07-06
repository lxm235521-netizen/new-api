import { api } from '@/lib/api'
import type { PluginPageContentResponse } from './types'

export async function getPluginPageContent(): Promise<PluginPageContentResponse> {
  const res = await api.get('/api/plugin_page_content')
  return res.data
}
