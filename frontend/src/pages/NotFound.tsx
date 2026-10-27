import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/Button";

export default function NotFoundPage() {
  const { t } = useTranslation();
  return (
    <div className="flex min-h-dvh items-center justify-center">
      <div className="text-center">
        <p className="text-7xl font-bold text-brand-200">404</p>
        <h1 className="mt-4 text-2xl font-bold text-slate-900 dark:text-slate-100">{t("notFound.title")}</h1>
        <p className="mt-2 text-slate-500 dark:text-slate-400">{t("notFound.message")}</p>
        <Link to="/">
          <Button className="mt-6">{t("notFound.backToDashboard")}</Button>
        </Link>
      </div>
    </div>
  );
}
