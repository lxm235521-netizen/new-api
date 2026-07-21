export interface OnlineUsePageContentResponse {
  success: boolean
  message?: string
  data?: string
}

export interface OnlineUsePageContentResult {
  content: string
  isLoaded: boolean
  isUrl: boolean
  isHtml: boolean
}
