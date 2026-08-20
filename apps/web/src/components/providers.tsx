"use client";

import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import { createContext, useContext, useEffect, useState } from "react";
import { api, ApiError, type Me } from "@/lib/api";
import { ToastProvider } from "@/components/ui";

export function Providers({ children }: { children: React.ReactNode }) {
  const [client] = useState(() => new QueryClient({ defaultOptions: { queries: { retry: (count, error) => !(error instanceof ApiError && error.status < 500) && count < 2, staleTime: 10_000, refetchOnWindowFocus: true } } }));
  return <QueryClientProvider client={client}><LocaleProvider><ToastProvider>{children}</ToastProvider></LocaleProvider></QueryClientProvider>;
}

type Locale = "kk" | "ru" | "en";
const copy: Record<Locale, Record<string, string>> = {
  en: {
    compose: "Compose", mail: "Mail", inbox: "Inbox", sent: "Sent", drafts: "Drafts", spam: "Spam", trash: "Trash", settings: "Settings", messages: "Messages",
    workspace: "Workspace", administration: "Administration", adminConsole: "Admin console", tasksReminders: "Tasks & Reminders", official: "Official", organization: "Organization",
    searchMessages: "Search messages", admin: "Admin", signOut: "Sign out", theme: "Theme", system: "System", light: "Light", dark: "Dark",
    dashboard: "Dashboard", organizations: "Organizations", domains: "Domains", mailboxes: "Mailboxes", aliases: "Aliases", groups: "Groups", apiKeys: "API Keys", smtp: "SMTP", webhooks: "Webhooks", securityCenter: "Security Center", audit: "Audit", team: "Team", overview: "Overview", infrastructure: "Infrastructure", developer: "Developer", security: "Security", backToMail: "Back to Mail", operations: "Operations", messageTrace: "Message Trace", infrastructureHealth: "Infrastructure",
    signInTitle: "Sign in to QazEra", signInHint: "Enter your work email and password to continue.", email: "Email", password: "Password", signIn: "Sign in", signingIn: "Signing in", quickAccess: "Quick access", quickHint: "Use a test profile to enter instantly.", administrator: "Administrator", mailUser: "Mail user", secureByDesign: "Secure by design", protectedWorkspace: "Your communications workspace, protected and under control.", protected: "Protected by secure sessions and rate limiting.", featureAccess: "Role-based access and secure sessions", featureAudit: "Auditable infrastructure operations", featureUnified: "Business mail and security in one workspace", invalidCredentials: "Invalid email or password.", tooManyAttempts: "Too many attempts. Try again in a few minutes.", serverUnavailable: "Could not sign in. Check that the server is running.",
    departments: "Departments", departmentsHint: "Create organization departments, appoint managers, and manage employee membership.", createDepartment: "Create department", departmentName: "Department name", description: "Description", manager: "Manager", notAssigned: "Not assigned", departmentCreated: "Department created", departmentSaved: "Department saved", departmentDeleted: "Department deleted", operationFailed: "Operation failed", creating: "Creating...", create: "Create", loadingDepartments: "Loading departments", noDepartments: "No departments yet", noDepartmentsHint: "Create the first department to organize your team.", noDescription: "No description", people: "people", members: "Members", noMembers: "No members", manage: "Manage", delete: "Delete", clearAll: "Clear all", selectAll: "Select all", saving: "Saving...", save: "Save", cancel: "Cancel", deleteDepartment: "Delete department?", deleteDepartmentHint: "Employees will remain in the organization without a department.", chooseDepartment: "Choose department", allDepartments: "All departments", searchRecipients: "Search by name, email, or department...", noPeopleFound: "No employees found", selectedRecipients: "Selected recipients", contacts: "Contacts", contactsHint: "Find coworkers by name, email, or department.", loadingContacts: "Loading contacts", write: "Write",
  },
  ru: {
    compose: "Написать", mail: "Почта", inbox: "Входящие", sent: "Отправленные", drafts: "Черновики", spam: "Спам", trash: "Корзина", settings: "Настройки", messages: "Сообщения",
    workspace: "Рабочее пространство", administration: "Администрирование", adminConsole: "Панель администратора", tasksReminders: "Задачи и напоминания", official: "Официальные", organization: "Организация",
    searchMessages: "Поиск писем", admin: "Админ", signOut: "Выйти", theme: "Тема", system: "Системная", light: "Светлая", dark: "Тёмная",
    dashboard: "Обзор", organizations: "Организации", domains: "Домены", mailboxes: "Почтовые ящики", aliases: "Алиасы", groups: "Группы", apiKeys: "API-ключи", smtp: "SMTP", webhooks: "Вебхуки", securityCenter: "Центр безопасности", audit: "Аудит", team: "Команда", overview: "Обзор", infrastructure: "Инфраструктура", developer: "Разработчикам", security: "Безопасность", backToMail: "Вернуться в почту", operations: "Операции", messageTrace: "Трассировка писем", infrastructureHealth: "Инфраструктура",
    signInTitle: "Войти в QazEra", signInHint: "Введите рабочую почту и пароль.", email: "Электронная почта", password: "Пароль", signIn: "Войти", signingIn: "Вход", quickAccess: "Быстрый вход", quickHint: "Выберите тестовый профиль для мгновенного входа.", administrator: "Администратор", mailUser: "Пользователь почты", secureByDesign: "Безопасность по умолчанию", protectedWorkspace: "Защищённое пространство для корпоративных коммуникаций.", protected: "Защищено безопасными сессиями и ограничением попыток входа.", featureAccess: "Ролевой доступ и защищённые сессии", featureAudit: "Контролируемые операции с инфраструктурой", featureUnified: "Почта и безопасность в одном пространстве", invalidCredentials: "Неверная почта или пароль.", tooManyAttempts: "Слишком много попыток. Повторите через несколько минут.", serverUnavailable: "Не удалось войти. Проверьте, запущен ли сервер.",
    departments: "Отделы", departmentsHint: "Создавайте отделы организации, назначайте руководителей и управляйте составом сотрудников.", createDepartment: "Создать отдел", departmentName: "Название отдела", description: "Описание", manager: "Руководитель", notAssigned: "Не назначен", departmentCreated: "Отдел создан", departmentSaved: "Отдел сохранён", departmentDeleted: "Отдел удалён", operationFailed: "Не удалось выполнить операцию", creating: "Создание...", create: "Создать", loadingDepartments: "Загрузка отделов", noDepartments: "Отделов пока нет", noDepartmentsHint: "Создайте первый отдел, чтобы организовать команду.", noDescription: "Без описания", people: "сотрудников", members: "Участники", noMembers: "Нет участников", manage: "Управлять", delete: "Удалить", clearAll: "Снять выбор", selectAll: "Выбрать всех", saving: "Сохранение...", save: "Сохранить", cancel: "Отмена", deleteDepartment: "Удалить отдел?", deleteDepartmentHint: "Сотрудники останутся в организации без привязки к отделу.", chooseDepartment: "Выбрать отдел", allDepartments: "Все отделы", searchRecipients: "Поиск по имени, email или отделу...", noPeopleFound: "Сотрудники не найдены", selectedRecipients: "Выбранные получатели", contacts: "Контакты", contactsHint: "Поиск коллег по имени, email или отделу.", loadingContacts: "Загрузка контактов", write: "Написать",
  },
  kk: {
    compose: "Хат жазу", mail: "Пошта", inbox: "Кіріс", sent: "Жіберілген", drafts: "Жобалар", spam: "Спам", trash: "Себет", settings: "Баптаулар", messages: "Хабарламалар",
    workspace: "Жұмыс кеңістігі", administration: "Әкімшілік", adminConsole: "Әкімші панелі", tasksReminders: "Тапсырмалар мен еске салулар", official: "Ресми хабарлар", organization: "Ұйым",
    searchMessages: "Хаттарды іздеу", admin: "Әкімші", signOut: "Шығу", theme: "Тақырып", system: "Жүйелік", light: "Ақ", dark: "Қараңғы",
    dashboard: "Шолу", organizations: "Ұйымдар", domains: "Домендер", mailboxes: "Пошта жәшіктері", aliases: "Алиастар", groups: "Топтар", apiKeys: "API кілттері", smtp: "SMTP", webhooks: "Вебхуктар", securityCenter: "Қауіпсіздік орталығы", audit: "Аудит", team: "Команда", overview: "Шолу", infrastructure: "Инфрақұрылым", developer: "Әзірлеуші", security: "Қауіпсіздік", backToMail: "Поштаға оралу", operations: "Операциялар", messageTrace: "Хат трассировкасы", infrastructureHealth: "Инфрақұрылым",
    signInTitle: "QazEra жүйесіне кіру", signInHint: "Жұмыс поштасы мен құпиясөзді енгізіңіз.", email: "Электрондық пошта", password: "Құпиясөз", signIn: "Кіру", signingIn: "Кіруде", quickAccess: "Жылдам кіру", quickHint: "Тест профилін таңдап, бірден кіріңіз.", administrator: "Әкімші", mailUser: "Пошта пайдаланушысы", secureByDesign: "Қауіпсіздік әуелден енгізілген", protectedWorkspace: "Корпоративтік байланыстарға арналған қорғалған жұмыс кеңістігі.", protected: "Қауіпсіз сессиялармен және кіру шектеулерімен қорғалған.", featureAccess: "Рөлдік қолжетімділік және қауіпсіз сессиялар", featureAudit: "Инфрақұрылым операцияларының аудиті", featureUnified: "Пошта мен қауіпсіздік бір кеңістікте", invalidCredentials: "Пошта немесе құпиясөз қате.", tooManyAttempts: "Әрекет тым көп. Бірнеше минуттан кейін қайталаңыз.", serverUnavailable: "Кіру мүмкін болмады. Сервердің жұмысын тексеріңіз.",
    departments: "Бөлімдер", departmentsHint: "Ұйым бөлімдерін құрыңыз, басшыларды тағайындаңыз және қызметкерлер құрамын басқарыңыз.", createDepartment: "Бөлім құру", departmentName: "Бөлім атауы", description: "Сипаттама", manager: "Басшы", notAssigned: "Тағайындалмаған", departmentCreated: "Бөлім құрылды", departmentSaved: "Бөлім сақталды", departmentDeleted: "Бөлім жойылды", operationFailed: "Әрекет орындалмады", creating: "Құрылуда...", create: "Құру", loadingDepartments: "Бөлімдер жүктелуде", noDepartments: "Бөлімдер әлі жоқ", noDepartmentsHint: "Команданы ұйымдастыру үшін бірінші бөлімді құрыңыз.", noDescription: "Сипаттамасыз", people: "қызметкер", members: "Қатысушылар", noMembers: "Қатысушылар жоқ", manage: "Басқару", delete: "Жою", clearAll: "Таңдауды тазарту", selectAll: "Барлығын таңдау", saving: "Сақталуда...", save: "Сақтау", cancel: "Бас тарту", deleteDepartment: "Бөлім жойылсын ба?", deleteDepartmentHint: "Қызметкерлер ұйымда бөлімсіз қалады.", chooseDepartment: "Бөлімді таңдау", allDepartments: "Барлық бөлімдер", searchRecipients: "Аты, email немесе бөлімі бойынша іздеу...", noPeopleFound: "Қызметкерлер табылмады", selectedRecipients: "Таңдалған алушылар", contacts: "Байланыстар", contactsHint: "Әріптестерді аты, email немесе бөлімі бойынша табыңыз.", loadingContacts: "Байланыстар жүктелуде", write: "Хат жазу",
  },
};
const LocaleContext = createContext<{ locale: Locale; setLocale: (locale: Locale) => void; t: (key: string) => string }>({ locale: "en", setLocale: () => {}, t: (key) => key });

function LocaleProvider({ children }: { children: React.ReactNode }) {
  const [locale, setLocale] = useState<Locale>(() => {
    if (typeof window === "undefined") return "en";
    const saved = localStorage.getItem("qazera-locale");
    return saved === "kk" || saved === "ru" ? saved : "en";
  });
  useEffect(() => { document.documentElement.lang = locale; localStorage.setItem("qazera-locale", locale); }, [locale]);
  const t = (key: string) => copy[locale][key] ?? copy.en[key] ?? key;
  return <LocaleContext.Provider value={{ locale, setLocale, t }}>{children}</LocaleContext.Provider>;
}

export function useI18n() { return useContext(LocaleContext); }

export function LocaleSwitcher({ className = "" }: { className?: string }) {
  const { locale, setLocale } = useI18n();
  return <div className={`inline-flex rounded-lg border border-border bg-surface p-0.5 ${className}`}>{(["kk", "ru", "en"] as Locale[]).map((item) => <button key={item} type="button" onClick={() => setLocale(item)} className={`h-7 rounded-md px-2 text-[11px] font-semibold uppercase transition-colors ${locale === item ? "bg-surface-elevated text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground"}`}>{item}</button>)}</div>;
}

export function useMe() {
  return useQuery<Me, ApiError>({ queryKey: ["me"], queryFn: () => api.get<Me>("/api/v1/me"), retry: false, staleTime: 60_000 });
}
