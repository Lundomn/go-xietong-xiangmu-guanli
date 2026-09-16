export default {
    crossDomain: ['true', '1'].indexOf(String(process.env.VUE_APP_CROSS_DOMAIN || '').toLowerCase()) !== -1, //是否开启跨域支持
    PROD_URL: process.env.VUE_APP_API_URL || '', //生产环境接口地址
    WS_URI: process.env.VUE_APP_WS_URI || '', //WebSocket地址
    HOME_PAGE: process.env.VUE_APP_HOME_PAGE || '/home',//主页路由
};
