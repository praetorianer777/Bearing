import { useNavigate } from "@tanstack/react-router";
import { type FormEvent, type ReactNode, useId, useState } from "react";
import { atLeast, useMe } from "@/api/auth";
import {
  useArchivePosition,
  useArchiveUnit,
  useCreatePerson,
  useCreatePosition,
  useCreateRole,
  useCreateUnit,
  useDeleteRole,
  useEndHolding,
  useGiveHolding,
  useLeave,
  useMoveUnit,
  useRemoveHolding,
  useRestoreUnit,
  useReturn,
  useSetReportsTo,
  useUpdatePerson,
  useUpdatePosition,
  useUpdateRole,
  useUpdateUnit,
} from "@/api/edit";
import type { Holding, OrgRole } from "@/api/people";
import type { Person, Position, Unit, UnitType } from "@/api/org";
import { Button, Field, FormError, fieldError } from "@/components/ui";
import { t, useLanguage } from "@/i18n";
import { today } from "./common";

/** Whether the person signed in may change the organisation. */
export function useEditor(): boolean {
  const role = useMe().data?.role;
  return !!role && atLeast(role, "editor");
}

/** The editors' part of a page: each change folded under its name until it is wanted. */
export function EditSection({ children }: { children: ReactNode }) {
  useLanguage();
  const id = useId();
  return (
    <section aria-labelledby={id} className="rounded-xl border border-border bg-surface p-5 text-sm">
      <h2 id={id} className="text-lg font-semibold">
        {t.edit.heading}
      </h2>
      <div className="mt-2 divide-y divide-border">{children}</div>
    </section>
  );
}

function Action({ title, children }: { title: string; children: (done: (message: string) => void) => ReactNode }) {
  const [message, setMessage] = useState("");
  return (
    <details className="group py-2" onToggle={() => setMessage("")}>
      <summary className="cursor-pointer font-medium text-ink">{title}</summary>
      <div className="mt-3 flex flex-col gap-3">
        {children(setMessage)}
        {message && (
          <p role="status" className="text-ink-muted">
            {message}
          </p>
        )}
      </div>
    </details>
  );
}

function Select({
  label,
  value,
  onChange,
  children,
  required,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  children: ReactNode;
  required?: boolean;
}) {
  const id = useId();
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={id} className="font-medium">
        {label}
      </label>
      <select
        id={id}
        required={required}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="h-9 rounded-md border border-border-strong bg-surface px-2 text-base text-ink"
      >
        {children}
      </select>
    </div>
  );
}

function Form({ onSubmit, error, pending, submit, children }: { onSubmit: () => void; error: unknown; pending: boolean; submit: string; children: ReactNode }) {
  return (
    <form
      onSubmit={(event: FormEvent) => {
        event.preventDefault();
        onSubmit();
      }}
      className="flex flex-col gap-3"
    >
      {children}
      <FormError error={error} />
      <Button type="submit" disabled={pending} className="self-start">
        {submit}
      </Button>
    </form>
  );
}

const byName = (a: { name: string }, b: { name: string }) => a.name.localeCompare(b.name);

/** The units inside a unit, on the tree as given, so a move cannot put a unit inside itself. */
function inside(units: Unit[], id: string): Set<string> {
  const out = new Set([id]);
  for (let grew = true; grew; ) {
    grew = false;
    for (const u of units) {
      if (u.parentId && out.has(u.parentId) && !out.has(u.id)) {
        out.add(u.id);
        grew = true;
      }
    }
  }
  return out;
}

export function UnitChanges({ unit, units, types }: { unit: Unit; units: Unit[]; types: UnitType[] }) {
  useLanguage();
  const update = useUpdateUnit(unit.id);
  const create = useCreateUnit();
  const position = useCreatePosition();
  const move = useMoveUnit();
  const archive = useArchiveUnit(unit.id);
  const restore = useRestoreUnit(unit.id);
  const [details, setDetails] = useState({ name: unit.name, shortName: unit.shortName ?? "", description: unit.description, typeId: unit.typeId });
  const [sub, setSub] = useState({ name: "", typeId: types[0]?.id ?? "", from: today() });
  const [post, setPost] = useState({ title: "", seats: "1", headOfUnit: false, from: today() });
  const blocked = inside(units, unit.id);
  const [moveTo, setMoveTo] = useState({ parentId: "", from: today() });
  const [archiveOn, setArchiveOn] = useState(today());
  const typeOptions = types.map((ty) => (
    <option key={ty.id} value={ty.id}>
      {ty.name}
    </option>
  ));
  return (
    <EditSection>
      <Action title={t.edit.unitDetails}>
        {(done) => (
          <Form
            onSubmit={() => update.mutate(details, { onSuccess: () => done(t.edit.saved) })}
            error={update.error}
            pending={update.isPending}
            submit={t.edit.save}
          >
            <Field
              label={t.edit.name}
              required
              value={details.name}
              onChange={(e) => setDetails({ ...details, name: e.target.value })}
              error={fieldError(update.error, "name")}
            />
            <Field
              label={t.edit.shortName}
              value={details.shortName}
              onChange={(e) => setDetails({ ...details, shortName: e.target.value })}
              error={fieldError(update.error, "shortName")}
            />
            <Field label={t.edit.description} value={details.description} onChange={(e) => setDetails({ ...details, description: e.target.value })} />
            <Select label={t.edit.type} value={details.typeId} onChange={(typeId) => setDetails({ ...details, typeId })}>
              {typeOptions}
            </Select>
          </Form>
        )}
      </Action>
      <Action title={t.edit.addSubUnit}>
        {(done) => (
          <Form
            onSubmit={() => create.mutate({ ...sub, parentId: unit.id }, { onSuccess: (made) => done(t.edit.added(made.name)) })}
            error={create.error}
            pending={create.isPending}
            submit={t.edit.add}
          >
            <Field
              label={t.edit.name}
              required
              value={sub.name}
              onChange={(e) => setSub({ ...sub, name: e.target.value })}
              error={fieldError(create.error, "name")}
            />
            <Select label={t.edit.type} required value={sub.typeId} onChange={(typeId) => setSub({ ...sub, typeId })}>
              {typeOptions}
            </Select>
            <Field
              label={t.edit.from}
              type="date"
              required
              value={sub.from}
              onChange={(e) => setSub({ ...sub, from: e.target.value })}
              error={fieldError(create.error, "from")}
            />
          </Form>
        )}
      </Action>
      <Action title={t.edit.addPosition}>
        {(done) => (
          <Form
            onSubmit={() =>
              position.mutate(
                { unitId: unit.id, title: post.title, seats: Number(post.seats), headOfUnit: post.headOfUnit || undefined, from: post.from },
                { onSuccess: (made) => done(t.edit.added(made.title)) },
              )
            }
            error={position.error}
            pending={position.isPending}
            submit={t.edit.add}
          >
            <Field
              label={t.edit.title}
              required
              value={post.title}
              onChange={(e) => setPost({ ...post, title: e.target.value })}
              error={fieldError(position.error, "title")}
            />
            <Field
              label={t.edit.seats}
              type="number"
              min={1}
              required
              value={post.seats}
              onChange={(e) => setPost({ ...post, seats: e.target.value })}
              error={fieldError(position.error, "seats")}
            />
            <label className="flex items-center gap-2">
              <input type="checkbox" checked={post.headOfUnit} onChange={(e) => setPost({ ...post, headOfUnit: e.target.checked })} />
              {t.edit.headOfUnit}
            </label>
            <Field
              label={t.edit.from}
              type="date"
              required
              value={post.from}
              onChange={(e) => setPost({ ...post, from: e.target.value })}
              error={fieldError(position.error, "from")}
            />
          </Form>
        )}
      </Action>
      <Action title={t.edit.moveUnit}>
        {(done) => (
          <Form
            onSubmit={() => move.mutate({ unitID: unit.id, ...moveTo }, { onSuccess: () => done(t.edit.moved(moveTo.from)) })}
            error={move.error}
            pending={move.isPending}
            submit={t.edit.move}
          >
            <Select label={t.edit.into} required value={moveTo.parentId} onChange={(parentId) => setMoveTo({ ...moveTo, parentId })}>
              <option value="">{t.edit.chooseUnit}</option>
              {units
                .filter((u) => !blocked.has(u.id))
                .sort(byName)
                .map((u) => (
                  <option key={u.id} value={u.id}>
                    {u.name}
                  </option>
                ))}
            </Select>
            <Field
              label={t.edit.from}
              type="date"
              required
              value={moveTo.from}
              onChange={(e) => setMoveTo({ ...moveTo, from: e.target.value })}
              error={fieldError(move.error, "from")}
            />
          </Form>
        )}
      </Action>
      {unit.archivedOn ? (
        <Action title={t.edit.restore}>
          {(done) => (
            <Form
              onSubmit={() => restore.mutate(undefined, { onSuccess: () => done(t.edit.restored) })}
              error={restore.error}
              pending={restore.isPending}
              submit={t.edit.restore}
            >
              <p className="text-ink-muted">{t.edit.restoreIntro}</p>
            </Form>
          )}
        </Action>
      ) : (
        <Action title={t.edit.archiveUnit}>
          {(done) => (
            <Form
              onSubmit={() => archive.mutate(archiveOn, { onSuccess: () => done(t.edit.archived(archiveOn)) })}
              error={archive.error}
              pending={archive.isPending}
              submit={t.edit.archive}
            >
              <Field
                label={t.edit.on}
                type="date"
                required
                value={archiveOn}
                onChange={(e) => setArchiveOn(e.target.value)}
                error={fieldError(archive.error, "on")}
              />
            </Form>
          )}
        </Action>
      )}
    </EditSection>
  );
}

export function PositionChanges({
  position,
  positions,
  people,
  holdings,
}: {
  position: Position;
  positions: Position[];
  people: Person[];
  holdings: Holding[];
}) {
  useLanguage();
  const update = useUpdatePosition(position.id);
  const reports = useSetReportsTo();
  const give = useGiveHolding();
  const end = useEndHolding();
  const remove = useRemoveHolding();
  const archive = useArchivePosition(position.id);
  const [details, setDetails] = useState({
    title: position.title,
    description: position.description,
    seats: String(position.seats),
    headOfUnit: position.headOfUnit,
  });
  const [line, setLine] = useState({ reportsToId: position.reportsToId ?? "", from: today() });
  const [holder, setHolder] = useState({ personId: "", share: "100", from: today(), to: "" });
  const [endOn, setEndOn] = useState(today());
  const [archiveOn, setArchiveOn] = useState(today());
  const names = new Map(people.map((p) => [p.id, p.name]));
  const current = holdings.filter((h) => h.to == null || h.to > today());
  return (
    <EditSection>
      <Action title={t.edit.positionDetails}>
        {(done) => (
          <Form
            onSubmit={() => update.mutate({ ...details, seats: Number(details.seats) }, { onSuccess: () => done(t.edit.saved) })}
            error={update.error}
            pending={update.isPending}
            submit={t.edit.save}
          >
            <Field
              label={t.edit.title}
              required
              value={details.title}
              onChange={(e) => setDetails({ ...details, title: e.target.value })}
              error={fieldError(update.error, "title")}
            />
            <Field label={t.edit.description} value={details.description} onChange={(e) => setDetails({ ...details, description: e.target.value })} />
            <Field
              label={t.edit.seats}
              type="number"
              min={1}
              required
              value={details.seats}
              onChange={(e) => setDetails({ ...details, seats: e.target.value })}
              error={fieldError(update.error, "seats")}
            />
            <label className="flex items-center gap-2">
              <input type="checkbox" checked={details.headOfUnit} onChange={(e) => setDetails({ ...details, headOfUnit: e.target.checked })} />
              {t.edit.headOfUnit}
            </label>
          </Form>
        )}
      </Action>
      <Action title={t.edit.reportsTo}>
        {(done) => (
          <Form
            onSubmit={() =>
              reports.mutate({ positionID: position.id, reportsToId: line.reportsToId || undefined, from: line.from }, { onSuccess: () => done(t.edit.saved) })
            }
            error={reports.error}
            pending={reports.isPending}
            submit={t.edit.save}
          >
            <Select label={t.edit.manager} value={line.reportsToId} onChange={(reportsToId) => setLine({ ...line, reportsToId })}>
              <option value="">{t.edit.nobody}</option>
              {positions
                .filter((p) => p.id !== position.id)
                .sort((a, b) => a.title.localeCompare(b.title))
                .map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.title}
                  </option>
                ))}
            </Select>
            <Field
              label={t.edit.from}
              type="date"
              required
              value={line.from}
              onChange={(e) => setLine({ ...line, from: e.target.value })}
              error={fieldError(reports.error, "from")}
            />
          </Form>
        )}
      </Action>
      <Action title={t.edit.giveHolding}>
        {(done) => (
          <Form
            onSubmit={() =>
              give.mutate(
                { personId: holder.personId, positionId: position.id, share: Number(holder.share), from: holder.from, to: holder.to || undefined },
                { onSuccess: () => done(t.edit.holds(names.get(holder.personId) ?? "", holder.from)) },
              )
            }
            error={give.error}
            pending={give.isPending}
            submit={t.edit.give}
          >
            <Select label={t.edit.person} required value={holder.personId} onChange={(personId) => setHolder({ ...holder, personId })}>
              <option value="">{t.edit.choosePerson}</option>
              {[...people].sort(byName).map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </Select>
            <Field
              label={t.edit.share}
              type="number"
              min={1}
              max={100}
              required
              value={holder.share}
              onChange={(e) => setHolder({ ...holder, share: e.target.value })}
              error={fieldError(give.error, "share")}
            />
            <Field
              label={t.edit.from}
              type="date"
              required
              value={holder.from}
              onChange={(e) => setHolder({ ...holder, from: e.target.value })}
              error={fieldError(give.error, "from")}
            />
            <Field
              label={t.edit.until}
              type="date"
              value={holder.to}
              onChange={(e) => setHolder({ ...holder, to: e.target.value })}
              error={fieldError(give.error, "to")}
            />
          </Form>
        )}
      </Action>
      {current.length > 0 && (
        <Action title={t.edit.endHoldings}>
          {(done) => (
            <div className="flex flex-col gap-3">
              <Field label={t.edit.on} type="date" required value={endOn} onChange={(e) => setEndOn(e.target.value)} />
              <ul className="space-y-2">
                {current.map((h) => {
                  const name = names.get(h.personId) ?? "";
                  const planned = h.from > today();
                  return (
                    <li key={h.id} className="flex flex-wrap items-center justify-between gap-2">
                      <span>
                        {name} <span className="text-ink-subtle">{t.person.span(h.from, h.to ?? null)}</span>
                      </span>
                      <span className="flex gap-2">
                        {!planned && (
                          <Button
                            variant="secondary"
                            disabled={end.isPending}
                            onClick={() => end.mutate({ holdingID: h.id, on: endOn }, { onSuccess: () => done(t.edit.ended(name, endOn)) })}
                            aria-label={t.edit.endFor(name)}
                          >
                            {t.edit.end}
                          </Button>
                        )}
                        <Button
                          variant="danger"
                          disabled={remove.isPending}
                          onClick={() => remove.mutate(h.id, { onSuccess: () => done(t.edit.removed(name)) })}
                          aria-label={t.edit.removeFor(name)}
                        >
                          {t.edit.remove}
                        </Button>
                      </span>
                    </li>
                  );
                })}
              </ul>
              <FormError error={end.error ?? remove.error} />
            </div>
          )}
        </Action>
      )}
      {!position.archivedOn && (
        <Action title={t.edit.archivePosition}>
          {(done) => (
            <Form
              onSubmit={() => archive.mutate(archiveOn, { onSuccess: () => done(t.edit.archived(archiveOn)) })}
              error={archive.error}
              pending={archive.isPending}
              submit={t.edit.archive}
            >
              <Field
                label={t.edit.on}
                type="date"
                required
                value={archiveOn}
                onChange={(e) => setArchiveOn(e.target.value)}
                error={fieldError(archive.error, "on")}
              />
            </Form>
          )}
        </Action>
      )}
    </EditSection>
  );
}

export function PersonChanges({ person }: { person: Person }) {
  useLanguage();
  const update = useUpdatePerson(person.id);
  const leave = useLeave(person.id);
  const back = useReturn(person.id);
  const [details, setDetails] = useState({ name: person.name, email: person.email ?? "", phone: person.phone ?? "", location: person.location ?? "" });
  const [leaveOn, setLeaveOn] = useState(today());
  return (
    <EditSection>
      <Action title={t.edit.personDetails}>
        {(done) => (
          <Form
            onSubmit={() => update.mutate(details, { onSuccess: () => done(t.edit.saved) })}
            error={update.error}
            pending={update.isPending}
            submit={t.edit.save}
          >
            <Field
              label={t.edit.name}
              required
              value={details.name}
              onChange={(e) => setDetails({ ...details, name: e.target.value })}
              error={fieldError(update.error, "name")}
            />
            <Field
              label={t.edit.email}
              type="email"
              value={details.email}
              onChange={(e) => setDetails({ ...details, email: e.target.value })}
              error={fieldError(update.error, "email")}
            />
            <Field
              label={t.edit.phone}
              value={details.phone}
              onChange={(e) => setDetails({ ...details, phone: e.target.value })}
              error={fieldError(update.error, "phone")}
            />
            <Field
              label={t.edit.location}
              value={details.location}
              onChange={(e) => setDetails({ ...details, location: e.target.value })}
              error={fieldError(update.error, "location")}
            />
          </Form>
        )}
      </Action>
      {person.leftOn ? (
        <Action title={t.edit.returnTitle}>
          {(done) => (
            <Form
              onSubmit={() => back.mutate(undefined, { onSuccess: () => done(t.edit.returned) })}
              error={back.error}
              pending={back.isPending}
              submit={t.edit.returnSubmit}
            >
              <p className="text-ink-muted">{t.edit.returnIntro}</p>
            </Form>
          )}
        </Action>
      ) : (
        <Action title={t.edit.leave}>
          {(done) => (
            <Form
              onSubmit={() => leave.mutate(leaveOn, { onSuccess: () => done(t.edit.left(leaveOn)) })}
              error={leave.error}
              pending={leave.isPending}
              submit={t.edit.leaveSubmit}
            >
              <p className="text-ink-muted">{t.edit.leaveIntro}</p>
              <Field
                label={t.edit.on}
                type="date"
                required
                value={leaveOn}
                onChange={(e) => setLeaveOn(e.target.value)}
                error={fieldError(leave.error, "on")}
              />
            </Form>
          )}
        </Action>
      )}
    </EditSection>
  );
}

export function RoleChanges({ role }: { role: OrgRole }) {
  useLanguage();
  const navigate = useNavigate();
  const update = useUpdateRole(role.id);
  const remove = useDeleteRole(role.id);
  const [details, setDetails] = useState({ name: role.name, description: role.description });
  return (
    <EditSection>
      <Action title={t.edit.roleDetails}>
        {(done) => (
          <Form
            onSubmit={() => update.mutate(details, { onSuccess: () => done(t.edit.saved) })}
            error={update.error}
            pending={update.isPending}
            submit={t.edit.save}
          >
            <Field
              label={t.edit.name}
              required
              value={details.name}
              onChange={(e) => setDetails({ ...details, name: e.target.value })}
              error={fieldError(update.error, "name")}
            />
            <Field label={t.edit.description} value={details.description} onChange={(e) => setDetails({ ...details, description: e.target.value })} />
          </Form>
        )}
      </Action>
      <Action title={t.edit.deleteRole}>
        {() => (
          <Form
            onSubmit={() => remove.mutate(undefined, { onSuccess: () => navigate({ to: "/roles" }) })}
            error={remove.error}
            pending={remove.isPending}
            submit={t.edit.deleteSubmit}
          >
            <p className="text-ink-muted">{t.edit.deleteRoleIntro}</p>
          </Form>
        )}
      </Action>
    </EditSection>
  );
}

/** Adds somebody to the organisation, external until an account or the realm links them, and opens their page. */
export function AddPerson() {
  useLanguage();
  const navigate = useNavigate();
  const create = useCreatePerson();
  const [details, setDetails] = useState({ name: "", email: "" });
  return (
    <EditSection>
      <Action title={t.edit.addPerson}>
        {() => (
          <Form
            onSubmit={() => create.mutate(details, { onSuccess: (made) => navigate({ to: "/people/$personId", params: { personId: made.id } }) })}
            error={create.error}
            pending={create.isPending}
            submit={t.edit.add}
          >
            <Field
              label={t.edit.name}
              required
              value={details.name}
              onChange={(e) => setDetails({ ...details, name: e.target.value })}
              error={fieldError(create.error, "name")}
            />
            <Field
              label={t.edit.email}
              type="email"
              value={details.email}
              onChange={(e) => setDetails({ ...details, email: e.target.value })}
              error={fieldError(create.error, "email")}
            />
          </Form>
        )}
      </Action>
    </EditSection>
  );
}

/** Adds a role, named by a key other tools keep, and opens its page. */
export function AddRole() {
  useLanguage();
  const navigate = useNavigate();
  const create = useCreateRole();
  const [details, setDetails] = useState({ name: "", key: "", description: "" });
  return (
    <EditSection>
      <Action title={t.edit.addRole}>
        {() => (
          <Form
            onSubmit={() => create.mutate(details, { onSuccess: (made) => navigate({ to: "/roles/$roleId", params: { roleId: made.id } }) })}
            error={create.error}
            pending={create.isPending}
            submit={t.edit.add}
          >
            <Field
              label={t.edit.name}
              required
              value={details.name}
              onChange={(e) => setDetails({ ...details, name: e.target.value })}
              error={fieldError(create.error, "name")}
            />
            <Field
              label={t.edit.key}
              required
              pattern="[a-z0-9][a-z0-9-]*"
              value={details.key}
              onChange={(e) => setDetails({ ...details, key: e.target.value })}
              error={fieldError(create.error, "key")}
            />
            <Field label={t.edit.description} value={details.description} onChange={(e) => setDetails({ ...details, description: e.target.value })} />
          </Form>
        )}
      </Action>
    </EditSection>
  );
}
