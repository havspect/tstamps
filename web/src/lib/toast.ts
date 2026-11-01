import { problemMessage } from './api'

type Variant = 'danger' | 'success' | 'warning'

const icons: Record<Variant, string> = {
  danger: 'circle-exclamation',
  success: 'circle-check',
  warning: 'triangle-exclamation',
}

// App.svelte mounts the one <wa-toast>. Look it up per call: the element is
// absent on the login page and in tests, and a new shell must not leave this
// module holding a stale node.
export function notify(message: string, options?: { variant?: Variant; duration?: number }): void {
  if (typeof document === 'undefined') return
  const toast = document.querySelector('wa-toast')
  if (!toast) return
  const variant = options?.variant
  void toast.create(message, {
    variant: variant ?? 'neutral',
    duration: options?.duration ?? (variant === 'danger' ? 8000 : 5000),
    icon: variant ? icons[variant] : undefined,
  })
}

export function notifyError(error: unknown, response?: Response): void {
  notify(problemMessage(error, response), { variant: 'danger' })
}
