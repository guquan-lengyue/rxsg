// 文案先抽用 legacy lang_zh_CN.js，错误键与后端 {"code"} 对齐。
export const zhCN = {
  appTitle: '热血三国',
  login: {
    title: '登录',
    passport: '账号',
    password: '密码',
    remember: '记住账号',
    submit: '进入游戏',
    submitting: '登录中…',
    announcement: '公告',
  },
  city: {
    logout: '退出登录',
    changeCity: '切换城池',
    loading: '加载中…',
    coordinate: '坐标',
    upgradeCountdown: '升级中',
  },
  resource: {
    food: '粮食',
    wood: '木材',
    rock: '石料',
    iron: '铁锭',
    gold: '黄金',
    people: '人口',
    morale: '民心',
  },
  errors: {
    invalid_user_auth: '登录已失效，请重新登录',
    invalid_user_pwd: '密码不正确',
    not_user_city: '该城池不属于当前用户',
    server_is_updating: '服务器维护中，请稍后再试',
    internal_error: '服务器开小差了，请稍后再试',
    network: '网络异常，请稍后重试',
  },
} as const

export type Language = typeof zhCN