import { Link } from 'react-router-dom'

export function ForbiddenPage() {
  return (
    <div className="min-h-screen flex items-center justify-center bg-background">
      <div className="text-center space-y-4">
        <h1 className="text-4xl font-bold">403</h1>
        <p className="text-muted-foreground">
          You don&apos;t have permission to access this page.
        </p>
        <Link to="/" className="text-primary hover:underline">
          Go back home
        </Link>
      </div>
    </div>
  )
}
