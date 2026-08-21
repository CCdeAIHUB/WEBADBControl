import { describe, expect, it } from 'vitest'

import { riskForOperation } from './risk'

describe('operation risk policy', () => {
  it('requires confirmation for destructive and privacy-sensitive operations', () => {
    // 场景：卸载、清除数据、短信和电话能力必须进入二次确认流程。
    for (const operation of ['package.uninstall', 'package.clear', 'sms.send', 'phone.call']) {
      expect(riskForOperation(operation).confirmationRequired).toBe(true)
    }
  })

  it('does not interrupt routine navigation controls', () => {
    expect(riskForOperation('input.home').confirmationRequired).toBe(false)
  })
})
