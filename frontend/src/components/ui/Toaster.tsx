import * as ToastPrimitive from "@radix-ui/react-toast";
import { cva, type VariantProps } from "class-variance-authority";
import { AlertCircle, AlertTriangle, CheckCircle2, Info, X } from "lucide-react";
import { useNavigate } from "@tanstack/react-router";
import { cn } from "@/lib/utils";
import { useToastStore, type ToastItem } from "@/store/toast.store";

export const toastVariants = cva(
  "pointer-events-auto relative flex w-full max-w-sm items-start gap-3 overflow-hidden " +
    "rounded-xl border p-3.5 shadow-lg backdrop-blur-xl " +
    "data-[state=open]:animate-slide-up " +
    "data-[swipe=cancel]:translate-x-0 data-[swipe=move]:translate-x-[var(--radix-toast-swipe-move-x)] " +
    "data-[swipe=end]:translate-x-[var(--radix-toast-swipe-end-x)] " +
    "transition-transform",
  {
    variants: {
      variant: {
        info: "border-slate-200/80 bg-white/95 text-slate-800 dark:border-white/10 dark:bg-slate-900/95 dark:text-slate-100",
        success:
          "border-emerald-200/80 bg-emerald-50/95 text-emerald-950 dark:border-emerald-500/25 dark:bg-emerald-950/90 dark:text-emerald-50",
        warning:
          "border-amber-200/80 bg-amber-50/95 text-amber-950 dark:border-amber-500/25 dark:bg-amber-950/90 dark:text-amber-50",
        error:
          "border-red-200/80 bg-red-50/95 text-red-950 dark:border-red-500/25 dark:bg-red-950/90 dark:text-red-50",
      },
    },
    defaultVariants: { variant: "info" },
  },
);

const ICONS = {
  info: Info,
  success: CheckCircle2,
  warning: AlertTriangle,
  error: AlertCircle,
} as const;

function ToastCard({
  item,
  onOpenChange,
}: {
  item: ToastItem;
  onOpenChange: (open: boolean) => void;
}) {
  const navigate = useNavigate();
  const variant = item.variant ?? "info";
  const Icon = ICONS[variant];

  function handleActivate() {
    if (item.link?.startsWith("/")) {
      navigate({ to: item.link });
      onOpenChange(false);
    }
  }

  return (
    <ToastPrimitive.Root
      duration={item.duration}
      onOpenChange={onOpenChange}
      className={cn(toastVariants({ variant }), item.link && "cursor-pointer")}
      onClick={item.link ? handleActivate : undefined}
    >
      <Icon size={16} className="mt-0.5 shrink-0 opacity-80" strokeWidth={2} />
      <div className="min-w-0 flex-1">
        <ToastPrimitive.Title className="text-sm font-semibold leading-snug">
          {item.title}
        </ToastPrimitive.Title>
        {item.description ? (
          <ToastPrimitive.Description className="mt-0.5 text-xs leading-relaxed opacity-80">
            {item.description}
          </ToastPrimitive.Description>
        ) : null}
      </div>
      <ToastPrimitive.Close
        className="rounded-lg p-1 opacity-50 transition hover:bg-black/5 hover:opacity-100 dark:hover:bg-white/10"
        aria-label="Dismiss"
        onClick={(e) => e.stopPropagation()}
      >
        <X size={14} />
      </ToastPrimitive.Close>
    </ToastPrimitive.Root>
  );
}

export type ToastVariantProps = VariantProps<typeof toastVariants>;

/** Global toast viewport — mount once inside the authenticated shell. */
export function Toaster() {
  const { toasts, dismiss } = useToastStore();

  return (
    <ToastPrimitive.Provider swipeDirection="right" duration={4_500}>
      {toasts.map((item) => (
        <ToastCard
          key={item.id}
          item={item}
          onOpenChange={(open) => {
            if (!open) dismiss(item.id);
          }}
        />
      ))}
      <ToastPrimitive.Viewport
        className="fixed bottom-4 right-4 z-[100] flex max-h-screen w-full max-w-sm flex-col gap-2 outline-none
                   p-0 sm:bottom-6 sm:right-6"
      />
    </ToastPrimitive.Provider>
  );
}
