package com.alibaba.opencodereview.idea.review

import com.alibaba.opencodereview.idea.messages.WebviewChannel

/**
 * 配置面板的宿主窗口，只负责窗口生命周期：创建、显露与销毁。
 * 消息处理在 [ConfigPanelRouter]。拆成接口是因为具体面板依赖前端就绪信号才能实现；在该信号到达之前，
 * 打开配置的请求退化成一条 IDE 通知而非静默无响应。
 */
interface ConfigPanelHost {

    /** 面板当前是否已经打开。 */
    val isOpen: Boolean

    /** 打开面板；已经打开时把它带到前台。 */
    fun open()

    /** 关闭面板。 */
    fun close()

    /** 面板的出站通道；面板没打开时实现方应当直接丢弃。 */
    val channel: WebviewChannel
}
