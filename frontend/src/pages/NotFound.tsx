import { Link } from "@tanstack/react-router";
import { Button } from "@/components/ui/Button";

export default function NotFoundPage() {
  return (
    <div className="flex min-h-dvh items-center justify-center">
      <div className="text-center">
        <p className="text-7xl font-bold text-brand-200">404</p>
        <h1 className="mt-4 text-2xl font-bold text-slate-900 dark:text-slate-100">Page not found</h1>
        <p className="mt-2 text-slate-500 dark:text-slate-400">The page you're looking for doesn't exist.</p>
        <Link to="/">
          <Button className="mt-6">Back to Dashboard</Button>
        </Link>
      </div>
    </div>
  );
}
