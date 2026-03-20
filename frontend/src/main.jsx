import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App.jsx'
import './index.css'

// React 18's createRoot API — replaces the old ReactDOM.render().
// StrictMode renders components twice in development to help catch side effects.
// It has no effect in production builds.
ReactDOM.createRoot(document.getElementById('root')).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
