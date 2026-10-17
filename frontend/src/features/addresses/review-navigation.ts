import { getJob, runSummary, type Job } from '../../api/client'
import { isReviewed } from './result-details'

function needsReview(job: Job, index: number) {
  return job.results[index].status !== 'matched' && !isReviewed(job.results[index], job.reviewed[index])
}

// Positions remain stable when a filtered CLI page shrinks after confirmation.
export async function nextReview(job: Job, after: number): Promise<{ job: Job; index: number } | null> {
  if (!job.local) {
    const later = job.results.findIndex((_, i) => i > after && needsReview(job, i))
    const index = later >= 0 ? later : job.results.findIndex((_, i) => needsReview(job, i))
    return index < 0 ? null : { job, index }
  }
  const later = job.local.rows.findIndex((row, i) => row.position > after && needsReview(job, i))
  if (later >= 0) return { job, index: later }

  // Search the pending queue, rather than downloading all intervening accepted rows.
  const first = await getJob(job.id, { offset: 0, reviewOnly: true })
  if (!first.local?.rows.length) return null
  let page = first
  if (first.local.rows.at(-1)!.position <= after) {
    const summary = await runSummary(job.id)
    let low = 1
    let high = Math.floor((summary.needs_review - 1) / 50)
    while (low <= high) {
      const middle = Math.floor((low + high) / 2)
      const candidate = await getJob(job.id, { offset: middle * 50, reviewOnly: true })
      if (candidate.local?.rows.length && candidate.local.rows.at(-1)!.position > after) {
        page = candidate
        high = middle - 1
      } else low = middle + 1
    }
  }
  const target = page.local!.rows.find(row => row.position > after) ?? first.local.rows[0]
  if (!job.local.reviewOnly) page = await getJob(job.id, { offset: Math.floor(target.position / 50) * 50, reviewOnly: false })
  const index = page.local!.rows.findIndex(row => row.position === target.position)
  return index < 0 ? null : { job: page, index }
}
