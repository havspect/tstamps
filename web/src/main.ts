import { mount } from 'svelte'
import '@awesome.me/webawesome/dist/styles/webawesome.css'
import '@awesome.me/webawesome/dist/components/badge/badge.js'
import '@awesome.me/webawesome/dist/components/button/button.js'
import '@awesome.me/webawesome/dist/components/callout/callout.js'
import '@awesome.me/webawesome/dist/components/checkbox/checkbox.js'
import '@awesome.me/webawesome/dist/components/dialog/dialog.js'
import '@awesome.me/webawesome/dist/components/icon/icon.js'
import { registerIconLibrary } from '@awesome.me/webawesome/dist/components/icon/library.js'
import { icons as systemIcons } from '@awesome.me/webawesome/dist/components/icon/library.system.js'
import '@awesome.me/webawesome/dist/components/input/input.js'
import '@awesome.me/webawesome/dist/components/option/option.js'
import '@awesome.me/webawesome/dist/components/page/page.js'
import '@awesome.me/webawesome/dist/components/scroller/scroller.js'
import '@awesome.me/webawesome/dist/components/select/select.js'
import '@awesome.me/webawesome/dist/components/tag/tag.js'
import '@awesome.me/webawesome/dist/components/textarea/textarea.js'
import './app.css'
import App from './App.svelte'

const extraIcons: Record<string, string> = {
  'arrow-right':
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 448 512"><path d="M438.6 278.6c12.5-12.5 12.5-32.8 0-45.3l-160-160c-12.5-12.5-32.8-12.5-45.3 0s-12.5 32.8 0 45.3L338.7 224 32 224c-17.7 0-32 14.3-32 32s14.3 32 32 32l306.7 0-105.4 105.4c-12.5 12.5-12.5 32.8 0 45.3s32.8 12.5 45.3 0l160-160z"/></svg>',
  'arrow-left':
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 448 512"><path d="M9.4 233.4c-12.5 12.5-12.5 32.8 0 45.3l160 160c12.5 12.5 32.8 12.5 45.3 0s12.5-32.8 0-45.3L109.3 288 416 288c17.7 0 32-14.3 32-32s-14.3-32-32-32l-306.7 0 105.4-105.4c12.5-12.5 12.5-32.8 0-45.3s-32.8-12.5-45.3 0l-160 160z"/></svg>',
  'arrow-up-right-from-square':
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512"><path d="M320 0c-17.7 0-32 14.3-32 32s14.3 32 32 32h82.7L201.4 265.4c-12.5 12.5-12.5 32.8 0 45.3s32.8 12.5 45.3 0L448 109.3V192c0 17.7 14.3 32 32 32s32-14.3 32-32V32c0-17.7-14.3-32-32-32zM80 32C35.8 32 0 67.8 0 112v320c0 44.2 35.8 80 80 80h320c44.2 0 80-35.8 80-80V320c0-17.7-14.3-32-32-32s-32 14.3-32 32v112c0 8.8-7.2 16-16 16H80c-8.8 0-16-7.2-16-16V112c0-8.8 7.2-16 16-16h112c17.7 0 32-14.3 32-32s-14.3-32-32-32z"/></svg>',
  stopwatch:
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 448 512"><path d="M176 0c-17.7 0-32 14.3-32 32s14.3 32 32 32h16v34.4C92.3 113.8 16 200 16 304c0 114.9 93.1 208 208 208s208-93.1 208-208c0-41.8-12.3-80.7-33.5-113.2l24.1-24.1c12.5-12.5 12.5-32.8 0-45.3s-32.8-12.5-45.3 0L355.7 143c-28.1-23-62.2-38.8-99.7-44.6V64h16c17.7 0 32-14.3 32-32S289.7 0 272 0zm72 192v128c0 13.3-10.7 24-24 24s-24-10.7-24-24V192c0-13.3 10.7-24 24-24s24 10.7 24 24z"/></svg>',
  terminal:
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 576 512"><path d="M9.4 86.6C-3.1 74.1-3.1 53.9 9.4 41.4s32.8-12.5 45.3 0l192 192c12.5 12.5 12.5 32.8 0 45.3l-192 192c-12.5 12.5-32.8 12.5-45.3 0s-12.5-32.8 0-45.3L178.7 256 9.4 86.6zM256 416l288 0c17.7 0 32 14.3 32 32s-14.3 32-32 32l-288 0c-17.7 0-32-14.3-32-32s14.3-32 32-32z"/></svg>',
}
extraIcons['external-link'] = extraIcons['arrow-up-right-from-square']

registerIconLibrary('default', {
  resolver: (name, _family = 'classic', variant = 'solid') => {
    const svg =
      extraIcons[name] ??
      systemIcons[variant]?.[name] ??
      systemIcons.solid?.[name] ??
      systemIcons.regular?.[name] ??
      systemIcons.regular?.['circle-question']
    if (svg) {
      return `data:image/svg+xml,${encodeURIComponent(svg)}`
    }
    return ''
  },
  mutator: (svg) => {
    if (!svg.hasAttribute('fill')) {
      svg.setAttribute('fill', 'currentColor')
    }
  },
})

export default mount(App, { target: document.getElementById('app')! })
