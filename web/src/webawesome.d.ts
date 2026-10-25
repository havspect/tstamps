// Web Awesome ships custom elements, not Svelte components. Reuse the native
// attribute types so wrappers can spread their props and keep consumer types.
import type {
  HTMLAttributes,
  HTMLButtonAttributes,
  HTMLInputAttributes,
  HTMLSelectAttributes,
  HTMLTextareaAttributes,
} from 'svelte/elements'

declare module 'svelte/elements' {
  interface SvelteHTMLElements {
    'wa-button': HTMLButtonAttributes & {
      variant?: 'neutral' | 'brand' | 'success' | 'warning' | 'danger'
      appearance?: 'accent' | 'filled' | 'outlined' | 'filled-outlined' | 'plain'
      loading?: boolean
      pill?: boolean
      size?: 'xs' | 's' | 'm' | 'l' | 'xl' | 'small' | 'medium' | 'large'
    }
    'wa-callout': HTMLAttributes<HTMLElement> & {
      variant?: 'brand' | 'neutral' | 'success' | 'warning' | 'danger'
      appearance?: 'accent' | 'filled' | 'outlined' | 'plain' | 'filled-outlined'
    }
    'wa-input': HTMLInputAttributes & {
      label?: string
      hint?: string
      'with-clear'?: boolean
    }
    'wa-select': HTMLSelectAttributes & {
      label?: string
      hint?: string
    }
    'wa-option': HTMLAttributes<HTMLElement> & {
      value?: string
      selected?: boolean
      disabled?: boolean
    }
    'wa-textarea': HTMLTextareaAttributes & {
      label?: string
      hint?: string
    }
    'wa-checkbox': HTMLInputAttributes & {
      hint?: string
    }
    'wa-dialog': HTMLAttributes<HTMLElement> & {
      label?: string
      open?: boolean
      'light-dismiss'?: boolean
      'onwa-after-show'?: (event: Event) => void
      'onwa-after-hide'?: (event: Event) => void
    }
  }
}

export {}
