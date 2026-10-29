import { type FormEvent, useState } from "react";
import { type UnitType, useCreateUnitType, useDeleteUnitType, useUnitTypes, useUpdateUnitType } from "@/api/units";
import { UNIT_ICONS, type UnitIconName } from "@/components/icons";
import { Button, Field, FormError, fieldError } from "@/components/ui";
import { t, useLanguage } from "@/i18n";
import { SettingsNav } from "./SettingsNav";

const ICON_NAMES = Object.keys(UNIT_ICONS) as UnitIconName[];

function TypeIcon({ name, colour }: { name: string; colour: string }) {
  const Glyph = UNIT_ICONS[name as UnitIconName] ?? UNIT_ICONS.unit;
  return (
    <span className="flex size-8 shrink-0 items-center justify-center rounded-md" style={{ color: colour, backgroundColor: `${colour}1f` }}>
      <Glyph />
    </span>
  );
}

function IconSelect({ value, onChange, label }: { value: string; onChange: (icon: string) => void; label: string }) {
  return (
    <select
      aria-label={label}
      value={value}
      onChange={(event) => onChange(event.target.value)}
      className="h-9 rounded-md border border-border-strong bg-surface px-2 text-sm text-ink"
    >
      {ICON_NAMES.map((icon) => (
        <option key={icon} value={icon}>
          {t.unitTypes.icons[icon]}
        </option>
      ))}
    </select>
  );
}

function TypeRow({ type }: { type: UnitType }) {
  const update = useUpdateUnitType();
  const remove = useDeleteUnitType();
  const [name, setName] = useState(type.name);
  const save = (change: Partial<UnitType>) => update.mutate({ unitTypeID: type.id, ...change });
  return (
    <li className="flex flex-wrap items-start gap-3 border-b border-border py-3 last:border-b-0">
      <TypeIcon name={type.icon} colour={type.colour} />
      <form
        className="flex min-w-0 flex-1 flex-col gap-1"
        onSubmit={(event) => {
          event.preventDefault();
          save({ name });
        }}
      >
        <input
          aria-label={`${t.forms.name}: ${type.name}`}
          value={name}
          onChange={(event) => setName(event.target.value)}
          onBlur={() => name !== type.name && save({ name })}
          aria-invalid={fieldError(update.error, "name") ? true : undefined}
          className="h-9 rounded-md border border-border-strong bg-surface px-2.5 text-sm text-ink aria-invalid:border-danger"
        />
        <span className="text-xs text-ink-subtle">{t.unitTypes.inUse(type.units)}</span>
      </form>
      <IconSelect value={type.icon} onChange={(icon) => save({ icon })} label={`${t.unitTypes.icon}: ${type.name}`} />
      <input
        type="color"
        aria-label={`${t.unitTypes.colour}: ${type.name}`}
        value={type.colour}
        onChange={(event) => save({ colour: event.target.value })}
        className="h-9 w-12 rounded-md border border-border-strong bg-surface"
      />
      <Button
        variant="danger"
        onClick={() => remove.mutate(type.id)}
        disabled={type.units > 0 || remove.isPending}
        aria-label={`${t.unitTypes.remove}: ${type.name}`}
      >
        {t.unitTypes.remove}
      </Button>
      <div className="basis-full empty:hidden">
        <FormError error={update.error ?? remove.error} />
        {fieldError(update.error, "name") && <p className="text-xs text-danger">{fieldError(update.error, "name")}</p>}
      </div>
    </li>
  );
}

function CreateForm() {
  const create = useCreateUnitType();
  const [icon, setIcon] = useState<string>("unit");
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    create.mutate({ name: String(data.get("name") ?? ""), icon, colour: String(data.get("colour") ?? "#6f685d") }, { onSuccess: () => form.reset() });
  };
  return (
    <section aria-labelledby="new-type-heading" className="rounded-xl border border-border bg-surface p-5">
      <h2 id="new-type-heading" className="text-lg font-semibold">
        {t.unitTypes.add}
      </h2>
      <form onSubmit={submit} className="mt-3 flex flex-wrap items-end gap-3" noValidate>
        <div className="min-w-48 flex-1">
          <Field label={t.forms.name} name="name" error={fieldError(create.error, "name")} />
        </div>
        <IconSelect value={icon} onChange={setIcon} label={t.unitTypes.icon} />
        <input
          type="color"
          name="colour"
          defaultValue="#6f685d"
          aria-label={t.unitTypes.colour}
          className="h-9 w-12 rounded-md border border-border-strong bg-surface"
        />
        <Button type="submit" disabled={create.isPending}>
          {t.unitTypes.addSubmit}
        </Button>
      </form>
    </section>
  );
}

export function UnitTypesPage() {
  useLanguage();
  const types = useUnitTypes();
  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-6 px-6 py-8">
      <SettingsNav />
      <div>
        <h1 className="text-2xl font-semibold">{t.unitTypes.title}</h1>
        <p className="mt-1 text-base text-ink-muted">{t.unitTypes.intro}</p>
      </div>
      {types.data?.length === 0 && <p className="text-sm text-ink-muted">{t.unitTypes.none}</p>}
      {!!types.data?.length && (
        <ul aria-label={t.unitTypes.title} className="rounded-xl border border-border bg-surface px-5">
          {types.data.map((type) => (
            <TypeRow key={type.id} type={type} />
          ))}
        </ul>
      )}
      <CreateForm />
    </div>
  );
}
