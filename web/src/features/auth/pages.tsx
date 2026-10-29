import { Link, useNavigate } from "@tanstack/react-router";
import { type FormEvent, useState } from "react";
import {
  ssoStartURL,
  useAcceptInvitation,
  useInstallation,
  useInvitationLink,
  useRequestPasswordReset,
  useResetPassword,
  useSetUp,
  useSignIn,
} from "@/api/auth";
import { Button, Field, FormError, fieldError } from "@/components/ui";
import { t, useLanguage } from "@/i18n";
import { AuthLayout } from "./AuthLayout";

/** Where to go after signing in: the page that asked, if it is one of ours. */
function safeNext(next: string | undefined): string {
  return next?.startsWith("/") && !next.startsWith("//") ? next : "/";
}

function formValues(event: FormEvent<HTMLFormElement>): Record<string, string> {
  event.preventDefault();
  return Object.fromEntries([...new FormData(event.currentTarget)].map(([k, v]) => [k, String(v)]));
}

export function SetUpPage() {
  useLanguage();
  const navigate = useNavigate();
  const setUp = useSetUp();
  const submit = (event: FormEvent<HTMLFormElement>) => {
    const v = formValues(event);
    setUp.mutate(
      { name: v.name ?? "", adminName: v.adminName ?? "", email: v.email ?? "", password: v.password ?? "" },
      { onSuccess: () => navigate({ to: "/" }) },
    );
  };
  return (
    <AuthLayout title={t.setup.title} intro={t.setup.intro}>
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <FormError error={setUp.error} />
        <Field label={t.setup.organisation} name="name" autoComplete="organization" required error={fieldError(setUp.error, "name")} />
        <Field label={t.setup.yourName} name="adminName" autoComplete="name" required error={fieldError(setUp.error, "adminName")} />
        <Field label={t.forms.email} name="email" type="email" autoComplete="email" required error={fieldError(setUp.error, "email")} />
        <Field
          label={t.forms.password}
          name="password"
          type="password"
          autoComplete="new-password"
          required
          placeholder={t.forms.passwordHint}
          error={fieldError(setUp.error, "password")}
        />
        <Button type="submit" disabled={setUp.isPending}>
          {t.setup.submit}
        </Button>
      </form>
    </AuthLayout>
  );
}

export function SignInPage({ next, sso }: { next?: string; sso?: string }) {
  useLanguage();
  const navigate = useNavigate();
  const signIn = useSignIn();
  const installation = useInstallation();
  const submit = (event: FormEvent<HTMLFormElement>) => {
    const v = formValues(event);
    signIn.mutate({ email: v.email ?? "", password: v.password ?? "" }, { onSuccess: () => navigate({ to: safeNext(next) }) });
  };
  return (
    <AuthLayout title={t.signIn.title} intro={installation.data?.name}>
      {sso && t.signIn.outcomes[sso] && (
        <p role="status" className="mb-4 rounded-md bg-accent-subtle px-3 py-2 text-sm text-ink">
          {t.signIn.outcomes[sso]}
        </p>
      )}
      {installation.data?.sso && (
        <div className="mb-4 flex flex-col gap-3">
          <a
            href={ssoStartURL(safeNext(next))}
            className="inline-flex h-9 items-center justify-center rounded-md bg-primary px-3 text-sm font-medium text-on-primary hover:bg-primary-hover"
          >
            {t.signIn.sso}
          </a>
          <p className="text-center text-xs text-ink-subtle">{t.signIn.or}</p>
        </div>
      )}
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <FormError error={signIn.error} />
        <Field label={t.forms.email} name="email" type="email" autoComplete="username" required />
        <Field label={t.forms.password} name="password" type="password" autoComplete="current-password" required />
        <Button type="submit" disabled={signIn.isPending}>
          {t.signIn.submit}
        </Button>
        {installation.data?.mail && (
          <Link to="/forgot-password" className="text-center text-sm text-accent hover:underline">
            {t.signIn.forgot}
          </Link>
        )}
      </form>
    </AuthLayout>
  );
}

export function ForgotPasswordPage() {
  useLanguage();
  const request = useRequestPasswordReset();
  const submit = (event: FormEvent<HTMLFormElement>) => request.mutate(formValues(event).email ?? "");
  return (
    <AuthLayout title={t.forgot.title} intro={request.isSuccess ? undefined : t.forgot.intro}>
      {request.isSuccess ? (
        <p role="status" className="text-sm text-ink">
          {t.forgot.sent}
        </p>
      ) : (
        <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
          <FormError error={request.error} />
          <Field label={t.forms.email} name="email" type="email" autoComplete="username" required />
          <Button type="submit" disabled={request.isPending}>
            {t.forgot.submit}
          </Button>
        </form>
      )}
      <Link to="/sign-in" className="mt-4 block text-center text-sm text-accent hover:underline">
        {t.forgot.back}
      </Link>
    </AuthLayout>
  );
}

export function ResetPasswordPage({ token }: { token: string }) {
  useLanguage();
  const reset = useResetPassword(token);
  const submit = (event: FormEvent<HTMLFormElement>) => reset.mutate(formValues(event).password ?? "");
  return (
    <AuthLayout title={t.reset.title}>
      {reset.isSuccess ? (
        <div className="flex flex-col gap-4">
          <p role="status" className="text-sm text-ink">
            {t.reset.done}
          </p>
          <Link to="/sign-in" className="text-sm text-accent hover:underline">
            {t.reset.signIn}
          </Link>
        </div>
      ) : (
        <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
          <FormError error={reset.error} />
          <Field
            label={t.forms.newPassword}
            name="password"
            type="password"
            autoComplete="new-password"
            required
            placeholder={t.forms.passwordHint}
            error={fieldError(reset.error, "password")}
          />
          <Button type="submit" disabled={reset.isPending}>
            {t.reset.submit}
          </Button>
        </form>
      )}
    </AuthLayout>
  );
}

export function AcceptInvitationPage({ token }: { token: string }) {
  useLanguage();
  const navigate = useNavigate();
  const link = useInvitationLink(token);
  const accept = useAcceptInvitation(token);
  const [done, setDone] = useState(false);
  if (link.isError) {
    return (
      <AuthLayout title={t.invitation.title}>
        <p role="alert" className="text-sm text-ink">
          {t.invitation.invalid}
        </p>
      </AuthLayout>
    );
  }
  const submit = (event: FormEvent<HTMLFormElement>) => {
    const v = formValues(event);
    accept.mutate(
      { name: v.name ?? "", password: v.password ?? "" },
      {
        onSuccess: () => {
          setDone(true);
          navigate({ to: "/" });
        },
      },
    );
  };
  return (
    <AuthLayout title={t.invitation.title} intro={link.data ? t.invitation.intro(link.data.email, t.roles[link.data.role]) : undefined}>
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <FormError error={accept.error} />
        <Field label={t.forms.name} name="name" autoComplete="name" required error={fieldError(accept.error, "name")} />
        <Field
          label={t.forms.password}
          name="password"
          type="password"
          autoComplete="new-password"
          required
          placeholder={t.forms.passwordHint}
          error={fieldError(accept.error, "password")}
        />
        <Button type="submit" disabled={accept.isPending || done || !link.data}>
          {t.invitation.submit}
        </Button>
      </form>
    </AuthLayout>
  );
}
