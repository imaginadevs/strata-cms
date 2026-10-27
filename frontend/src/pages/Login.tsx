import { useMemo, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { motion } from "framer-motion";
import { useTranslation } from "react-i18next";
import { post } from "@/lib/api";
import { useAuthStore } from "@/store/auth.store";
import { Button } from "@/components/ui/Button";
import type { TokenResponse } from "@/types";

type FormData = {
  email: string;
  password: string;
};

export default function LoginPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { setSession } = useAuthStore();
  const [error, setError] = useState<string | null>(null);

  const schema = useMemo(
    () =>
      z.object({
        email: z.string().email(t("auth.invalidEmail")),
        password: z.string().min(1, t("auth.passwordRequired")),
      }),
    [t],
  );

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<FormData>({ resolver: zodResolver(schema) });

  const onSubmit = async (data: FormData) => {
    setError(null);
    try {
      const response = await post<TokenResponse>("/auth/login", data);
      setSession(response);
      navigate({ to: "/" });
    } catch (err) {
      setError((err as Error).message ?? t("auth.loginFailed"));
    }
  };

  return (
    <div className="flex min-h-dvh items-center justify-center bg-app-bg p-4">
      <motion.div
        initial={{ opacity: 0, y: 16 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.25 }}
        className="w-full max-w-md"
      >
        {/* Card */}
        <div className="glass-card-elevated p-8">
          {/* Logo */}
          <div className="mb-8 flex flex-col items-center gap-3">
            <div className="flex h-14 w-14 items-center justify-center rounded-2xl
                            bg-brand-600 shadow-glass-lg">
              <span className="text-2xl font-bold text-white">C</span>
            </div>
            <div className="text-center">
              <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">{t("auth.welcomeBack")}</h1>
              <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">{t("auth.signInSubtitle")}</p>
            </div>
          </div>

          <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
            {/* Email */}
            <div>
              <label className="mb-1.5 block text-sm font-medium text-slate-700 dark:text-slate-200">
                {t("common.email")}
              </label>
              <input
                {...register("email")}
                type="email"
                autoComplete="email"
                placeholder={t("auth.emailPlaceholder")}
                className="input"
              />
              {errors.email && (
                <p className="mt-1.5 text-xs text-red-500 dark:text-red-400">{errors.email.message}</p>
              )}
            </div>

            {/* Password */}
            <div>
              <label className="mb-1.5 block text-sm font-medium text-slate-700 dark:text-slate-200">
                {t("common.password")}
              </label>
              <input
                {...register("password")}
                type="password"
                autoComplete="current-password"
                placeholder="••••••••"
                className="input"
              />
              {errors.password && (
                <p className="mt-1.5 text-xs text-red-500 dark:text-red-400">{errors.password.message}</p>
              )}
            </div>

            {/* Global error */}
            {error && (
              <div className="rounded-xl bg-red-50 dark:bg-red-500/10 px-4 py-3 text-sm text-red-600 dark:text-red-400">
                {error}
              </div>
            )}

            <Button type="submit" className="mt-2 w-full" loading={isSubmitting}>
              {t("auth.signIn")}
            </Button>
          </form>

          {/* SSO divider — shown only when SSO is enabled */}
          <div className="mt-6 flex items-center gap-3">
            <div className="h-px flex-1 bg-slate-200 dark:bg-white/10" />
            <span className="text-xs text-slate-400">{t("auth.orContinueWith")}</span>
            <div className="h-px flex-1 bg-slate-200 dark:bg-white/10" />
          </div>
          <a
            href="/api/auth/sso"
            className="mt-3 flex w-full items-center justify-center gap-2 rounded-xl
                       border border-slate-200 dark:border-white/10 bg-white/80 dark:bg-white/5 px-4 py-2.5 text-sm
                       font-medium text-slate-700 dark:text-slate-200 shadow-sm transition hover:shadow"
          >
            {t("auth.ssoLogto")}
          </a>
        </div>
      </motion.div>
    </div>
  );
}
