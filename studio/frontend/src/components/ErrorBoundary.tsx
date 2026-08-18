import React, { Component, ErrorInfo, ReactNode } from 'react'
import { AlertCircle, RefreshCw } from 'lucide-react'

interface Props {
  children: ReactNode
}

interface State {
  hasError: boolean
  error: Error | null
}

export class ErrorBoundary extends Component<Props, State> {
  public state: State = {
    hasError: false,
    error: null,
  }

  public static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error }
  }

  public componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error('Uncaught component error:', error, errorInfo)
  }

  public render() {
    if (this.state.hasError) {
      return (
        <div className="p-8 m-6 rounded-xl bg-red-500/10 border border-red-500/30 text-red-600 dark:text-red-400 space-y-4">
          <div className="flex items-center gap-2 font-semibold text-base">
            <AlertCircle className="w-5 h-5" />
            <span>Component Rendering Error</span>
          </div>
          <p className="text-xs font-mono bg-zinc-950/80 p-3 rounded text-red-300 overflow-auto">
            {this.state.error?.message || 'An unexpected error occurred while rendering.'}
          </p>
          <button
            onClick={() => this.setState({ hasError: false, error: null })}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-red-600 hover:bg-red-500 text-white text-xs font-medium cursor-pointer"
          >
            <RefreshCw className="w-3.5 h-3.5" />
            <span>Retry Rendering</span>
          </button>
        </div>
      )
    }

    return this.props.children
  }
}
