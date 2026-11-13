import { createDecisionService, DIRECT_STREAM_FORMATS } from './decisionService'
import { fetchTranscodeDecision } from './fetchDecision'
export { detectBrowserProfile } from './browserProfile'
export { DIRECT_STREAM_FORMATS }

export const decisionService = createDecisionService(fetchTranscodeDecision)
