import { Component, ErrorInfo, ReactNode } from 'react';
import { AlertTriangle, RefreshCw, Copy, Check, Home } from 'lucide-react';

interface Props {
  children: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
  errorInfo: ErrorInfo | null;
  copied: boolean;
}

export class ErrorBoundary extends Component<Props, State> {
  public state: State = {
    hasError: false,
    error: null,
    errorInfo: null,
    copied: false,
  };

  public static getDerivedStateFromError(error: Error): Partial<State> {
    return { hasError: true, error };
  }

  public componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error('[ErrorBoundary] Uncaught application error:', error, errorInfo);
    this.setState({ errorInfo });
  }

  private handleReload = () => {
    window.location.reload();
  };

  private handleGoHome = () => {
    window.location.href = '/';
  };

  private handleCopyDetails = () => {
    const { error, errorInfo } = this.state;
    const text = `OpenLocalCRM Error Report\n-------------------------\nTime: ${new Date().toISOString()}\nURL: ${window.location.href}\nError: ${error?.message || 'Unknown'}\nStack: ${error?.stack || 'N/A'}\nComponent Stack: ${errorInfo?.componentStack || 'N/A'}`;
    
    navigator.clipboard.writeText(text).then(() => {
      this.setState({ copied: true });
      setTimeout(() => this.setState({ copied: false }), 2500);
    }).catch(() => {});
  };

  public render() {
    if (this.state.hasError) {
      const { error, errorInfo, copied } = this.state;

      return (
        <div className="min-h-screen bg-slate-950 text-slate-100 flex items-center justify-center p-4">
          <div className="max-w-xl w-full bg-slate-900 border border-slate-800 rounded-2xl shadow-2xl p-6 sm:p-8">
            <div className="flex items-center gap-3 mb-4">
              <div className="w-12 h-12 rounded-xl bg-rose-500/10 border border-rose-500/20 flex items-center justify-center text-rose-400 shrink-0">
                <AlertTriangle className="w-6 h-6" />
              </div>
              <div>
                <h1 className="text-xl font-bold text-slate-100">Ein unerwarteter Fehler ist aufgetreten</h1>
                <p className="text-sm text-slate-400">OpenLocalCRM hat die Benutzeroberfläche isoliert.</p>
              </div>
            </div>

            <div className="bg-slate-950/80 border border-slate-800/80 rounded-xl p-4 mb-6">
              <p className="text-sm font-medium text-rose-400 font-mono break-words">
                {error?.message || 'Unbekannter JavaScript-Fehler'}
              </p>
            </div>

            <div className="flex flex-wrap gap-3 mb-6">
              <button
                onClick={this.handleReload}
                className="flex items-center gap-2 px-4 py-2.5 rounded-xl bg-emerald-600 hover:bg-emerald-500 text-white font-medium text-sm transition-colors shadow-sm"
              >
                <RefreshCw className="w-4 h-4" />
                Seite neu laden
              </button>

              <button
                onClick={this.handleGoHome}
                className="flex items-center gap-2 px-4 py-2.5 rounded-xl bg-slate-800 hover:bg-slate-700 text-slate-200 font-medium text-sm transition-colors border border-slate-700"
              >
                <Home className="w-4 h-4" />
                Zum Dashboard
              </button>

              <button
                onClick={this.handleCopyDetails}
                className="flex items-center gap-2 px-4 py-2.5 rounded-xl bg-slate-800/60 hover:bg-slate-800 text-slate-300 font-medium text-sm transition-colors border border-slate-700/60 ml-auto"
              >
                {copied ? <Check className="w-4 h-4 text-emerald-400" /> : <Copy className="w-4 h-4" />}
                {copied ? 'Kopiert!' : 'Diagnose kopieren'}
              </button>
            </div>

            {errorInfo?.componentStack && (
              <details className="text-xs text-slate-500 group">
                <summary className="cursor-pointer hover:text-slate-400 select-none">
                  Technische Details & Stack anzeigen
                </summary>
                <pre className="mt-3 p-3 bg-slate-950 rounded-lg overflow-x-auto text-[11px] font-mono text-slate-400 border border-slate-800/60 max-h-48 overflow-y-auto">
                  {error?.stack}
                  {'\n\nComponent Stack:\n'}
                  {errorInfo.componentStack}
                </pre>
              </details>
            )}
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
