const { chromium } = require('playwright');

function jsonOK(data) {
  return JSON.stringify({ code: 0, message: 'ok', data });
}

function parseISO(raw) {
  if (!raw) return null;
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return null;
  return date;
}

function collectTasks(taskMap) {
  return Object.values(taskMap).flat();
}

(async () => {
  const now = new Date();
  const todayDue = new Date(now);
  todayDue.setHours(14, 0, 0, 0);
  const nextDue = new Date(now);
  nextDue.setDate(nextDue.getDate() + 3);
  nextDue.setHours(16, 30, 0, 0);

  const state = {
    user: {
      id: 7,
      username: 'playwright-user',
      email: 'playwright@example.com',
      avatar_url: '',
    },
    projects: [
      { id: 1, name: 'Inbox', color: '#e0b882' },
      { id: 2, name: 'Work', color: '#4f86f7' },
    ],
    tasksByProject: {
      1: [
        {
          id: 101,
          title: 'Prepare design review',
          project_id: 1,
          status: 'todo',
          priority: 2,
          due_at: todayDue.toISOString(),
          content_md: 'Initial content',
          members: [],
          user_id: 7,
        },
      ],
      2: [
        {
          id: 201,
          title: 'Write backend patch',
          project_id: 2,
          status: 'todo',
          priority: 1,
          due_at: nextDue.toISOString(),
          content_md: 'Backend details',
          members: [],
          user_id: 7,
        },
      ],
    },
    nextTaskID: 300,
  };

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext();
  const page = await context.newPage();

  await page.addInitScript(() => {
    localStorage.setItem('access_token', 'fake-token');
  });

  await page.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const method = request.method();
    const url = new URL(request.url());
    const path = url.pathname.replace('/api/v1', '');

    const send = (body, status = 200) =>
      route.fulfill({
        status,
        headers: { 'content-type': 'application/json' },
        body,
      });

    const allTasks = collectTasks(state.tasksByProject);

    if (path === '/users/me' && method === 'GET') {
      return send(jsonOK(state.user));
    }

    if (path === '/projects' && method === 'GET') {
      const name = (url.searchParams.get('name') || '').toLowerCase();
      const list = state.projects.filter((project) =>
        project.name.toLowerCase().includes(name),
      );
      return send(jsonOK({ list, total: list.length, page: 1, size: 100 }));
    }

    if (path === '/projects' && method === 'POST') {
      const body = request.postDataJSON();
      const nextID = state.projects.length + 1;
      const project = {
        id: nextID,
        name: body.name || `List ${nextID}`,
        color: body.color || '#e0b882',
      };
      state.projects.unshift(project);
      state.tasksByProject[nextID] = [];
      return send(jsonOK(project));
    }

    if (path.match(/^\/projects\/\d+$/) && method === 'GET') {
      const projectID = Number(path.split('/')[2]);
      const project = state.projects.find((item) => item.id === projectID);
      if (!project) return send(JSON.stringify({ code: 4004, message: 'not found' }), 404);
      return send(jsonOK(project));
    }

    if (path.match(/^\/projects\/\d+$/) && method === 'PATCH') {
      const projectID = Number(path.split('/')[2]);
      const body = request.postDataJSON();
      state.projects = state.projects.map((project) =>
        project.id === projectID ? { ...project, ...body } : project,
      );
      return send(jsonOK({}));
    }

    if (path.match(/^\/projects\/\d+$/) && method === 'DELETE') {
      const projectID = Number(path.split('/')[2]);
      state.projects = state.projects.filter((project) => project.id !== projectID);
      delete state.tasksByProject[projectID];
      return send(jsonOK({}));
    }

    if (path === '/tasks/me' && method === 'GET') {
      const dueStart = parseISO(url.searchParams.get('due_start'));
      const dueEnd = parseISO(url.searchParams.get('due_end'));
      const statusFilter = url.searchParams.get('status');
      let list = allTasks;
      if (statusFilter) {
        list = list.filter((task) => task.status === statusFilter);
      }
      if (dueStart) {
        list = list.filter((task) => task.due_at && new Date(task.due_at) >= dueStart);
      }
      if (dueEnd) {
        list = list.filter((task) => task.due_at && new Date(task.due_at) <= dueEnd);
      }
      return send(jsonOK({ list, total: list.length, page: 1, size: 200 }));
    }

    if (path === '/tasks' && method === 'GET') {
      const projectID = Number(url.searchParams.get('project_id'));
      const list = state.tasksByProject[projectID] || [];
      return send(jsonOK({ list, total: list.length, page: 1, size: 200 }));
    }

    if (path === '/tasks' && method === 'POST') {
      const body = request.postDataJSON();
      const projectID = Number(body.project_id);
      const task = {
        id: state.nextTaskID++,
        title: body.title || 'New Task',
        project_id: projectID,
        status: body.status || 'todo',
        priority: body.priority || 3,
        due_at: body.due_at || null,
        content_md: body.content_md || '',
        members: [],
        user_id: 7,
      };
      if (!state.tasksByProject[projectID]) state.tasksByProject[projectID] = [];
      state.tasksByProject[projectID].unshift(task);
      return send(jsonOK(task));
    }

    if (path.match(/^\/projects\/\d+\/tasks\/\d+$/) && method === 'PATCH') {
      const chunks = path.split('/');
      const projectID = Number(chunks[2]);
      const taskID = Number(chunks[4]);
      const body = request.postDataJSON();
      const list = state.tasksByProject[projectID] || [];
      const index = list.findIndex((task) => task.id === taskID);
      if (index >= 0) {
        const task = { ...list[index] };
        if (Object.prototype.hasOwnProperty.call(body, 'clear_due_at') && body.clear_due_at) {
          task.due_at = null;
        }
        Object.assign(task, body);
        delete task.clear_due_at;
        list[index] = task;
      }
      return send(jsonOK({}));
    }

    if (path.match(/^\/tasks\/\d+$/) && method === 'DELETE') {
      const taskID = Number(path.split('/')[2]);
      for (const key of Object.keys(state.tasksByProject)) {
        state.tasksByProject[key] = state.tasksByProject[key].filter((task) => task.id !== taskID);
      }
      return send(jsonOK({}));
    }

    if (path.match(/^\/projects\/\d+\/tasks\/\d+\/members$/) && method === 'POST') {
      return send(jsonOK({}));
    }

    if (path.match(/^\/projects\/\d+\/tasks\/\d+\/members$/) && method === 'DELETE') {
      return send(jsonOK({}));
    }

    if (path === '/logout' && method === 'POST') {
      return send(jsonOK({}));
    }

    return send(jsonOK({}));
  });

  await page.goto('http://127.0.0.1:5173/tasks/me', { waitUntil: 'networkidle' });
  await page.waitForSelector('text=Today');

  await page.click('text=Done');
  await page.waitForTimeout(200);

  await page.click('text=Next 7 Days');
  await page.waitForSelector('h1:has-text("Next 7 Days")');

  await page.click('text=Calendar');
  await page.waitForSelector('h1');
  await page.click('[data-testid="calendar-view-week"]');
  await page.waitForSelector('.yq-cal-week-grid');
  await page.click('[data-testid="calendar-view-day"]');
  await page.waitForSelector('.yq-cal-day-view');
  await page.click('[data-testid="calendar-view-agenda"]');
  await page.waitForSelector('.yq-cal-agenda, .yq-empty-state');

  await page.click('text=Inbox');
  await page.waitForSelector('h1:has-text("Inbox")');

  await page.fill('input[placeholder="Add a task to this list"]', 'Playwright created task');
  await page.click('button:has-text("Add Task")');
  await page.waitForSelector('text=Playwright created task');

  await page.click('button:has-text("Details")');
  await page.waitForSelector('button:has-text("Edit")');
  await page.click('button:has-text("Edit")');
  await page.waitForSelector('label:has-text("Description (Markdown)")');
  await page.fill('textarea', '## Updated by Playwright');
  await page.click('button:has-text("Save")');
  await page.waitForTimeout(300);

  await page.screenshot({ path: 'playwright-ui-smoke.png', fullPage: true });
  console.log('Playwright smoke test completed. Screenshot: playwright-ui-smoke.png');

  await browser.close();
})().catch((error) => {
  console.error(error);
  process.exit(1);
});
