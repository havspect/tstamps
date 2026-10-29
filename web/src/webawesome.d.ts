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
    'wa-input': Omit<HTMLInputAttributes, 'size'> & {
      label?: string
      hint?: string
      'with-clear'?: boolean
      size?: 'xs' | 's' | 'm' | 'l' | 'xl' | 'small' | 'medium' | 'large'
    }
    'wa-select': Omit<HTMLSelectAttributes, 'size'> & {
      label?: string
      hint?: string
      size?: 'xs' | 's' | 'm' | 'l' | 'xl' | 'small' | 'medium' | 'large'
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
    'wa-checkbox': Omit<HTMLInputAttributes, 'size'> & {
      hint?: string
      size?: 'xs' | 's' | 'm' | 'l' | 'xl' | 'small' | 'medium' | 'large'
    }
    'wa-dialog': HTMLAttributes<HTMLElement> & {
      label?: string
      open?: boolean
      'light-dismiss'?: boolean
      'onwa-after-show'?: (event: Event) => void
      'onwa-after-hide'?: (event: Event) => void
    }
    'wa-page': HTMLAttributes<HTMLElement> & {
      'mobile-breakpoint'?: number | string
      view?: 'mobile' | 'desktop'
    }
    'wa-icon': HTMLAttributes<HTMLElement> & {
      name?: string
      label?: string
      variant?: string
      family?: string
      src?: string
      size?: string
    }
    'wa-scroller': HTMLAttributes<HTMLElement>
    'wa-tag': HTMLAttributes<HTMLElement> & {
      variant?: 'neutral' | 'brand' | 'success' | 'warning' | 'danger'
      size?: 'small' | 'medium' | 'large'
      pill?: boolean
      removable?: boolean
    }
    'wa-badge': HTMLAttributes<HTMLElement> & {
      variant?: 'neutral' | 'brand' | 'success' | 'warning' | 'danger'
      pill?: boolean
      pulse?: boolean
    }
  }
}

export {}
