import { useEffect, useState } from 'react'
import { getPluginPageContent } from '../api'
import type { PluginPageContentResult } from '../types'

const STORAGE_KEY = 'plugin_page_content'

export function usePluginPageContent(): PluginPageContentResult {
  const [content, setContent] = useState<string>('')
  const [isLoaded, setIsLoaded] = useState(false)

  useEffect(() => {
    let mounted = true

    const loadContent = async () => {
      const cached = localStorage.getItem(STORAGE_KEY)
      if (cached && mounted) {
        setContent(cached)
      }

      try {
        const response = await getPluginPageContent()
        const { success, data } = response

        if (!mounted) return

        if (success && data) {
          setContent(data)
          localStorage.setItem(STORAGE_KEY, data)
        } else {
          setContent('')
          localStorage.removeItem(STORAGE_KEY)
        }
      } catch (error) {
        if (!mounted) return
        console.error('Failed to load plugin page content:', error)
      } finally {
        if (mounted) {
          setIsLoaded(true)
        }
      }
    }

    loadContent()

    return () => {
      mounted = false
    }
  }, [])

  let isUrl = false
  try {
    const url = new URL(content)
    isUrl = url.protocol === 'http:' || url.protocol === 'https:'
  } catch {
    // not a URL
  }

  const isHtml =
    !isUrl &&
    (/^\s*<!DOCTYPE\s+html/i.test(content.trim()) ||
      /^\s*<html[\s>]/i.test(content.trim()))

  return { content, isLoaded, isUrl, isHtml }
}
