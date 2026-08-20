"use client";

import Link from "next/link";
import { useMe } from "@/components/providers";
import { PageLoader, cx } from "@/components/ui";

const SECTIONS = [
  {
    number: "01",
    title: "Mail",
    text: "Inbox, drafts, search, reply flow, and attachments organized like a newsroom desk.",
    href: "/mail/inbox",
  },
  {
    number: "02",
    title: "Security",
    text: "Quarantine, blocks, audit trails, and policy review in a controlled editorial space.",
    href: "/admin/security",
  },
  {
    number: "03",
    title: "Control",
    text: "Organizations, domains, users, mailboxes, APIs, and webhooks in one operations layer.",
    href: "/admin",
  },
];

export default function Home() {
  const me = useMe();

  if (me.isLoading) {
    return (
      <main className="newsprint-texture flex min-h-screen items-center justify-center px-4 py-6">
        <PageLoader label="QazEra" />
      </main>
    );
  }

  const isAdmin = me.data?.permissions.includes("users.manage") ?? false;

  return (
    <main className="newsprint-texture min-h-screen bg-[#f9f9f7]">
      <div className="mx-auto max-w-screen-xl px-4 py-4 lg:px-6 lg:py-6">
        <header className="border-b-4 border-[#111111] pb-4">
          <div className="flex flex-wrap items-end justify-between gap-4">
            <div>
              <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">
                Vol. 1 | August 20, 2026 | QazEra Edition
              </p>
              <h1 className="mt-3 font-serif text-[clamp(3.5rem,10vw,8rem)] leading-[0.9] tracking-tighter text-[#111111]">
                QazEra
              </h1>
            </div>
            <div className="max-w-sm text-right">
              <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#111111]">
                Secure communications cloud
              </p>
              <p className="mt-2 text-sm leading-6 text-[#525252] font-body">
                A disciplined front page for mail, security, and administration.
              </p>
            </div>
          </div>
        </header>

        <section className="grid gap-0 border-x border-b-4 border-[#111111] lg:grid-cols-12">
          <div className="border-b border-[#111111] bg-[#f9f9f7] p-6 lg:col-span-8 lg:border-b-0 lg:border-r">
            <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">Lead Story</p>
            <p className="mt-4 max-w-3xl text-lg leading-8 text-[#111111] font-body text-justify">
              <span className="float-left mr-3 mt-2 font-serif text-7xl leading-none text-[#cc0000]">
                Q
              </span>
              qazEra is built like a morning paper: direct, structured, and easy to scan. Every
              control, message, and workflow is framed by visible borders, measured hierarchy, and
              editorial clarity.
            </p>

            <div className="mt-8 grid gap-0 border border-[#111111] md:grid-cols-3">
              {SECTIONS.map((section, index) => (
                <Link
                  key={section.title}
                  href={section.href}
                  className={cx(
                    "group border-b border-[#111111] p-5 transition-colors hover:bg-[#111111] hover:text-[#f9f9f7] md:border-b-0",
                    index < 2 && "md:border-r",
                  )}
                >
                  <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000] group-hover:text-[#f9f9f7]">
                    {section.number}
                  </p>
                  <h2 className="mt-4 font-serif text-3xl tracking-tight">{section.title}</h2>
                  <p className="mt-3 text-sm leading-6 font-body">{section.text}</p>
                </Link>
              ))}
            </div>
          </div>

          <aside className="bg-[#e5e5e0] p-6 lg:col-span-4">
            <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#111111]">
              Edition Notes
            </p>
            <div className="mt-4 space-y-4">
              <div className="border border-[#111111] bg-[#f9f9f7] p-4">
                <p className="font-serif text-2xl tracking-tight">Open the workspace</p>
                <p className="mt-2 text-sm leading-6 text-[#525252] font-body">
                  Continue to the inbox, or enter the control room if your role allows it.
                </p>
                <div className="mt-5 flex flex-wrap gap-2">
                  <Link
                    href={me.data ? "/mail/inbox" : "/login"}
                    className="inline-flex min-h-[44px] items-center justify-center border border-[#111111] bg-[#111111] px-4 text-xs font-bold uppercase tracking-[0.25em] text-[#f9f9f7] transition-colors hover:bg-white hover:text-[#111111]"
                  >
                    {me.data ? "Open inbox" : "Sign in"}
                  </Link>
                  {isAdmin && (
                    <Link
                      href="/admin"
                      className="inline-flex min-h-[44px] items-center justify-center border border-[#111111] bg-transparent px-4 text-xs font-bold uppercase tracking-[0.25em] text-[#111111] transition-colors hover:bg-[#111111] hover:text-[#f9f9f7]"
                    >
                      Admin
                    </Link>
                  )}
                </div>
              </div>

              <div className="border border-[#111111] bg-[#f9f9f7] p-4">
                <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">Status</p>
                <p className="mt-3 text-sm leading-6 font-body">
                  {me.data
                    ? `Signed in as ${me.data.display_name || me.data.email}.`
                    : "You are not signed in. Use the login page to enter the system."}
                </p>
              </div>

              <div className="border border-[#111111] bg-[#111111] p-4 text-[#f9f9f7]">
                <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">
                  04. Journal
                </p>
                <p className="mt-3 text-sm leading-6 font-body">
                  The interface intentionally reads like a front page, not a dashboard. That is the
                  point of the system.
                </p>
              </div>
            </div>
          </aside>
        </section>
      </div>
    </main>
  );
}
