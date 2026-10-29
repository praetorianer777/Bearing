import { type FormEvent, useState } from "react";
import { atLeast, useMe, useUpdateMe } from "@/api/auth";
import { Button, Field, FormError, fieldError } from "@/components/ui";
import { t, useLanguage } from "@/i18n";

export function ProfilePage() {
  useLanguage();
  const me = useMe().data;
  const update = useUpdateMe();
  const [name, setName] = useState(me?.name ?? "");
  if (!me) return null;
  const editable = atLeast(me.role, "member");
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    update.mutate({ name });
  };
  return (
    <div className="mx-auto flex max-w-2xl flex-col gap-6 px-6 py-8">
      <div>
        <h1 className="text-2xl font-semibold">{t.profile.title}</h1>
        <p className="mt-1 text-base text-ink-muted">{t.profile.intro}</p>
      </div>
      <form onSubmit={submit} className="flex flex-col gap-3 rounded-xl border border-border bg-surface p-5" noValidate>
        <Field
          label={t.forms.name}
          name="name"
          value={name}
          onChange={(event) => setName(event.target.value)}
          readOnly={!editable}
          error={fieldError(update.error, "name")}
        />
        <Field label={t.forms.email} name="email" value={me.email} readOnly />
        <p className="text-sm text-ink-muted">
          {me.role === me.ownRole ? t.profile.role(t.roles[me.role]) : t.profile.throughGroup(t.roles[me.role], t.roles[me.ownRole])}
        </p>
        {editable ? (
          <div className="flex items-center gap-3">
            <Button type="submit" disabled={update.isPending}>
              {t.profile.save}
            </Button>
            {update.isSuccess && (
              <span role="status" className="text-sm text-ink-muted">
                {t.profile.saved}
              </span>
            )}
          </div>
        ) : (
          <p className="text-sm text-ink-subtle">{t.profile.readOnly}</p>
        )}
        <FormError error={update.error} />
      </form>
    </div>
  );
}
