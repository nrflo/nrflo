import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setConsoleChatYolo } from './consoleChats'
import * as client from './client'

vi.mock('./client')

describe('setConsoleChatYolo', () => {
  beforeEach(() => vi.clearAllMocks())

  it('on=true POSTs to the yolo endpoint', async () => {
    vi.mocked(client.apiPost).mockResolvedValue(undefined)

    await setConsoleChatYolo('s 1', true)

    expect(client.apiPost).toHaveBeenCalledWith('/api/v1/console/chats/s%201/yolo')
    expect(client.apiDelete).not.toHaveBeenCalled()
  })

  it('on=false DELETEs the yolo endpoint', async () => {
    vi.mocked(client.apiDelete).mockResolvedValue(undefined)

    await setConsoleChatYolo('s1', false)

    expect(client.apiDelete).toHaveBeenCalledWith('/api/v1/console/chats/s1/yolo')
    expect(client.apiPost).not.toHaveBeenCalled()
  })
})
