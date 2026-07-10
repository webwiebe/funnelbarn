// JSX typings for the hosted IAMBarn web components. These custom elements are
// registered at runtime by the IAMBarn widget bundle (loaded via
// useIambarnWidget); TypeScript's strict mode otherwise rejects the unknown
// intrinsic tags. Attributes mirror the IAMBarn setup document.
import type { DetailedHTMLProps, HTMLAttributes } from 'react'

type IambarnElement<Extra> = DetailedHTMLProps<HTMLAttributes<HTMLElement>, HTMLElement> & Extra

declare global {
  namespace JSX {
    interface IntrinsicElements {
      'iambarn-user-menu': IambarnElement<{
        'server-url'?: string
        'client-id'?: string
        'account-href'?: string
        'post-logout-redirect-uri'?: string
        'show-email'?: boolean | string
      }>
      'iambarn-profile': IambarnElement<{
        'server-url'?: string
      }>
      'iambarn-avatar': IambarnElement<{
        'server-url'?: string
        size?: number | string
      }>
      'iambarn-user-badge': IambarnElement<{
        'server-url'?: string
        'show-email'?: boolean | string
      }>
      'iambarn-logout-button': IambarnElement<{
        'server-url'?: string
        'client-id'?: string
        'post-logout-redirect-uri'?: string
        label?: string
      }>
    }
  }
}
