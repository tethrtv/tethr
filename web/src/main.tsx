import '@fontsource-variable/archivo/wdth.css'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { ComingSoon } from './ComingSoon'
import './base.css'

const root = document.getElementById('root')
if (root) {
  createRoot(root).render(
    <StrictMode>
      <ComingSoon />
    </StrictMode>,
  )
}
