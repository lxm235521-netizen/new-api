export interface PluginPageContentResponse {
  success: boolean
  message?: string
  data?: string
}

export interface PluginPageContentResult {
  content: string
  isLoaded: boolean
  isUrl: boolean
  isHtml: boolean
}
