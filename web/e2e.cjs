const puppeteer = require('puppeteer');

(async () => {
    let browser;
    let page;
    try {
        console.log('Starting end-to-end UI tests...');

        // Attempting to connect to the existing DevTools port 9222 first
        try {
            console.log('Trying to connect to existing Chrome instance on port 9222...');
            browser = await puppeteer.connect({
                browserURL: 'http://127.0.0.1:9222'
            });
            console.log('Connected to existing browser.');
        } catch (e) {
            console.log('Could not connect to existing browser, launching headless...');
            browser = await puppeteer.launch({
                headless: "new",
                args: ['--no-sandbox', '--disable-setuid-sandbox']
            });
        }

        page = await browser.newPage();
        const BASE_URL = 'http://localhost:5173';

        // Helper to clear and type
        const clearAndType = async (selector, text) => {
            const input = await page.waitForSelector(selector);
            await input.click({ clickCount: 3 });
            await page.keyboard.press('Backspace');
            await input.type(text, { delay: 30 });
        };

        // 1. Register User
        console.log('Testing Registration...');
        await page.goto(`${BASE_URL}/register`);
        await new Promise(r => setTimeout(r, 2000)); // wait for React to mount

        // Switch lang to EN
        try {
            await page.select('.yq-lang-select', 'en');
            await new Promise(r => setTimeout(r, 1000));
        } catch (e) { }

        const testUser = `testuser_${Date.now()}`;
        await clearAndType('input[name="email"]', `${testUser}@example.com`);
        await clearAndType('input[name="username"]', testUser);
        await clearAndType('input[name="password"]', 'password123');
        await clearAndType('input[name="confirm_password"]', 'password123');

        const fs = require('fs');
        fs.writeFileSync('dummy.jpg', 'dummy content');
        const fileInput = await page.$('input[type="file"]');
        await fileInput.uploadFile('dummy.jpg');

        await page.click('button[type="submit"]');

        console.log('Waiting for redirect to login page...');
        await new Promise(r => setTimeout(r, 3000));

        // Check if still on register (error occurred)
        const url = page.url();
        if (url.includes('/register')) {
            const errorText = await page.evaluate(() => {
                const err = document.querySelector('.yq-auth-error');
                return err ? err.textContent : 'No visible error';
            });
            throw new Error(`Registration failed: ${errorText}`);
        }

        // 2. Login
        console.log('Testing Login...');
        await page.waitForSelector('input[name="username"]'); // means we are on login
        await clearAndType('input[name="username"]', testUser);
        await clearAndType('input[name="password"]', 'password123');
        await page.click('button[type="submit"]');

        // 3. Main Dashboard & Project Creation
        console.log('Testing Dashboard & Project Creation...');
        await page.waitForSelector('.yq-project-grid', { timeout: 15000 });
        await new Promise(r => setTimeout(r, 1500));

        const buttons = await page.$$('button');
        for (let btn of buttons) {
            const text = await page.evaluate(el => el.textContent, btn);
            if (text.includes('Create Project') || text.includes('新建项目')) {
                await btn.click();
                break;
            }
        }

        await page.waitForSelector('.yq-modal-content input');
        await page.type('.yq-modal-content input', 'My Auto Test Project', { delay: 50 });
        await page.click('.yq-color-option:nth-child(3)');

        const modalButtons = await page.$$('.yq-modal-footer button');
        await modalButtons[1].click();

        await new Promise(r => setTimeout(r, 2000));

        // 4. Click into project
        await page.waitForSelector('.yq-project-card');
        await page.click('.yq-project-card');

        // 5. Create Task
        console.log('Testing Task Creation...');
        await page.waitForSelector('.yq-task-list');
        await new Promise(r => setTimeout(r, 1500));

        const newTskBtns = await page.$$('button');
        for (let btn of newTskBtns) {
            const text = await page.evaluate(el => el.textContent, btn);
            if (text.includes('New Task') || text.includes('新建任务')) {
                await btn.click();
                break;
            }
        }

        await page.waitForSelector('.yq-modal-content input');
        await page.type('.yq-modal-content input', 'My Auto Test Task', { delay: 50 });

        const mdlTkBtns = await page.$$('.yq-modal-footer button');
        await mdlTkBtns[1].click();

        await new Promise(r => setTimeout(r, 2000));

        // 6. Edit Task
        console.log('Testing Task Details...');
        await page.waitForSelector('.yq-task-item');
        await page.click('.yq-task-item');

        await page.waitForSelector('.yq-task-panel.open');
        await new Promise(r => setTimeout(r, 1500));

        const panelBtns = await page.$$('.yq-panel-actions button');
        for (let btn of panelBtns) {
            const text = await page.evaluate(el => el.textContent, btn);
            if (text.includes('Edit') || text.includes('编辑')) {
                await btn.click();
                break;
            }
        }

        await page.waitForSelector('.yq-panel-edit-form textarea');
        await page.type('.yq-panel-edit-form textarea', '# Task Heading\nAutomated description', { delay: 10 });

        const saveBtns = await page.$$('.yq-panel-actions button');
        for (let btn of saveBtns) {
            const text = await page.evaluate(el => el.textContent, btn);
            if (text.includes('Save') || text.includes('保存')) {
                await btn.click();
                break;
            }
        }

        await new Promise(r => setTimeout(r, 2000));

        // 7. Profile editing
        console.log('Testing Profile...');
        await page.click('header .yq-header-actions a[href="/profile"]');

        await page.waitForSelector('.yq-profile-card');
        await new Promise(r => setTimeout(r, 1500));
        const profileBtns = await page.$$('button');
        for (let btn of profileBtns) {
            const text = await page.evaluate(el => el.textContent, btn);
            if (text.includes('Edit') || text.includes('编辑')) {
                await btn.click();
                break;
            }
        }

        await page.waitForSelector('input[type="email"]');
        const usernameInput = await page.$$('input');
        await usernameInput[1].click({ clickCount: 3 });
        await usernameInput[1].type(`${testUser}_updated`, { delay: 50 });

        const saveProfBtns = await page.$$('.yq-profile-form-actions button');
        await saveProfBtns[1].click();
        await new Promise(r => setTimeout(r, 2000));

        console.log('✅ All UI flows completed successfully!');

    } catch (error) {
        console.error('❌ E2E Test Failed:', error);
        if (page) {
            await page.screenshot({ path: 'e2e-failure.png', fullPage: true });
            console.log('Saved failure screenshot to e2e-failure.png');
        }
    } finally {
        if (browser) await browser.close();
    }
})();
