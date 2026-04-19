import axios from 'axios';

const client = axios.create({
    baseURL: '/api/v1',
    timeout: 10000,
});

client.interceptors.request.use(
    (config) => {
        const token = localStorage.getItem('access_token');
        if (token) {
            config.headers.Authorization = `Bearer ${token}`;
        }
        return config;
    },
    (error) => Promise.reject(error),
);

client.interceptors.response.use(
    (response) => {
        const res = response.data;
        if (res.code === 0) return res.data;

        const errorMsg = res.message || '\u672a\u77e5\u9519\u8bef';
        return Promise.reject(new Error(errorMsg));
    },
    (error) => {
        if (error.response) {
            const status = error.response.status;

            if (status === 401) {
                localStorage.removeItem('access_token');
                if (window.location.pathname !== '/login' && window.location.pathname !== '/register') {
                    window.location.href = '/login';
                }
                return Promise.reject(new Error('\u5f53\u524d\u8d26\u53f7\u65e0\u6743\u9650\u8bbf\u95ee\u6216\u767b\u5f55\u5df2\u8fc7\u671f'));
            }

            if (status >= 500) {
                const dateStr = new Date().toISOString().replace(/[-:T.]/g, '').slice(0, 14);
                return Promise.reject(new Error(`\u64cd\u4f5c\u5931\u8d25\uff0c\u8bf7\u7a0d\u540e\u91cd\u8bd5\u3002\u53c2\u8003\u7801\uff1aE${dateStr}`));
            }

            const serverMsg = error.response.data?.message || '\u8bf7\u6c42\u5931\u8d25\uff0c\u8bf7\u68c0\u67e5\u8f93\u5165\u53c2\u6570';
            return Promise.reject(new Error(serverMsg));
        }

        return Promise.reject(new Error('\u7f51\u7edc\u8bf7\u6c42\u5931\u8d25\uff0c\u8bf7\u68c0\u67e5\u8fde\u63a5'));
    },
);

export default client;
