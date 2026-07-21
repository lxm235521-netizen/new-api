import { createFileRoute } from '@tanstack/react-router'
import { OnlineUse } from '@/features/online-use'

export const Route = createFileRoute('/online-use')({
  component: OnlineUse,
})
