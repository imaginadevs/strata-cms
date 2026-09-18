import { cn } from "@/lib/utils";

interface Props {
  className?: string;
}

export function PageSpinner({ className }: Props) {
  return (
    <div className={cn("flex h-dvh w-full items-center justify-center", className)}>
      <div className="flex flex-col items-center gap-4">
        <div className="h-10 w-10 animate-spin rounded-full border-4 border-brand-200 border-t-brand-600" />
        <span className="text-sm text-slate-500 dark:text-slate-400">Loading…</span>
      </div>
    </div>
  );
}
