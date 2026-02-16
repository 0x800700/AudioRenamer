<script>
  import {
    SelectFolder,
    GenerateTemplateRenames,
    FetchAndMatchTracks,
    RenameMatchedTracks,
    ParseFilenamesWithAI,
  } from "../wailsjs/go/main/App";
  import { onMount } from "svelte";
  import logo from "./assets/ProBablyWorks.png";

  let localTracks = [];
  let processedTracks = [];
  let bandcampUrl = "";
  let notification = "";
  let isLoading = false;
  let folderPath = "";
  let apiKey = localStorage.getItem("openai_api_key") || "";
  let showSettings = false;

  async function selectFolder() {
    try {
      isLoading = true;
      notification = "Scanning folder...";
      const result = await SelectFolder();
      localTracks = result || [];

      if (localTracks.length > 0) {
        const firstTrackPath = localTracks[0].path;
        // Simple heuristic to get folder path
        const lastSeparatorIndex = Math.max(
          firstTrackPath.lastIndexOf("/"),
          firstTrackPath.lastIndexOf("\\"),
        );
        folderPath = firstTrackPath.substring(0, lastSeparatorIndex);
        notification = `Found ${localTracks.length} local files.`;

        processedTracks = localTracks.map((track) => ({
          localPath: track.path,
          originalName: track.originalName,
          proposedNewName: track.originalName,
          confidence: 0,
          status: "Original",
        }));
      } else {
        folderPath = "";
        notification = "No audio files found in the selected directory.";
        processedTracks = [];
      }
    } catch (error) {
      handleError(error);
    } finally {
      isLoading = false;
    }
  }

  let templateFormat = "Track. Artist - Title";

  async function generateFromTemplate() {
    try {
      isLoading = true;
      notification = "Generating names from template...";
      processedTracks =
        (await GenerateTemplateRenames(localTracks, templateFormat)) || [];
      notification = `Generated ${processedTracks.length} names. Review and rename.`;
    } catch (error) {
      handleError(error);
    } finally {
      isLoading = false;
    }
  }

  async function fetchAndMatch() {
    if (!bandcampUrl) {
      notification = "Please enter a Bandcamp URL.";
      return;
    }
    try {
      isLoading = true;
      notification = "Fetching data and matching files...";
      processedTracks =
        (await FetchAndMatchTracks(bandcampUrl, localTracks)) || [];
      notification = `Matched ${processedTracks.length} tracks. Review and rename.`;
    } catch (error) {
      handleError(error);
    } finally {
      isLoading = false;
    }
  }

  async function parseWithAI() {
    if (!apiKey) {
      notification = "Please set your API Key in settings first.";
      showSettings = true;
      return;
    }
    try {
      isLoading = true;
      notification = "Asking AI to parse filenames...";

      const filenames = localTracks.map((t) => t.originalName);
      const aiResults = await ParseFilenamesWithAI(filenames, apiKey);

      // Map AI results back to processedTracks
      processedTracks = localTracks.map((track) => {
        const aiMatch = aiResults.find(
          (r) => r.original_filename === track.originalName,
        );
        let proposedName = track.originalName;
        let status = "AI Parsed";
        let confidence = 0.9; // Assume high confidence if AI returns it

        if (aiMatch) {
          // Construct new name: "01. Artist - Title.ext"
          const ext = track.originalName.split(".").pop();
          const trackNum = aiMatch.track_number
            ? aiMatch.track_number.padStart(2, "0")
            : "00";
          proposedName = `${trackNum}. ${aiMatch.artist} - ${aiMatch.title}.${ext}`;
        } else {
          status = "AI Failed";
          confidence = 0.1;
        }

        return {
          localPath: track.path,
          originalName: track.originalName,
          proposedNewName: proposedName,
          confidence: confidence,
          status: status,
        };
      });

      notification = `AI parsing complete. Review and rename.`;
    } catch (error) {
      handleError(error);
    } finally {
      isLoading = false;
    }
  }

  async function renameFiles() {
    try {
      isLoading = true;
      notification = "Renaming files...";
      const result = await RenameMatchedTracks(processedTracks);
      notification = result;
      // Reset state after renaming
      localTracks = [];
      processedTracks = [];
      folderPath = "";
    } catch (error) {
      handleError(error);
    } finally {
      isLoading = false;
    }
  }

  function handleProposedNameChange(event, index) {
    processedTracks[index].proposedNewName = event.target.value;
  }

  function handleError(error) {
    console.error(error);
    notification = `Error: ${error.message || error}`;
  }

  function saveSettings() {
    localStorage.setItem("openai_api_key", apiKey);
    showSettings = false;
    notification = "Settings saved.";
  }

  function getConfidenceColor(confidence) {
    if (confidence < 0.4) return "confidence-low";
    if (confidence < 0.8) return "confidence-mid";
    return "confidence-high";
  }
</script>

<main
  class="min-h-screen flex flex-col bg-app text-ink overflow-hidden"
>
  <div
    class="flex-1 flex flex-col min-h-0 max-w-7xl mx-auto w-full p-4 md:p-8 space-y-8"
  >
    <!-- Header -->
    <header
      class="flex-none flex flex-wrap gap-4 justify-between items-center border-b border-soft pb-6"
    >
      <div class="flex items-center gap-6">
        <div>
          <h1
            class="text-3xl md:text-4xl font-semibold bg-gradient-to-r from-teal-700 to-amber-600 bg-clip-text text-transparent"
          >
            AudioRenamer
          </h1>
          <p class="text-muted text-sm mt-1">
            Smart audio file organization
          </p>
        </div>

        <button
          on:click={() => (showSettings = !showSettings)}
          class="btn btn-ghost text-sm"
          title="Settings"
        >
          <svg
            xmlns="http://www.w3.org/2000/svg"
            class="h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"
            />
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
            />
          </svg>
          <span class="text-xs font-semibold tracking-widest">API-KEY</span>
        </button>
      </div>

      <div class="flex flex-col items-end">
        <img src={logo} alt="ProBably Works" class="h-24 object-contain" />
      </div>
    </header>

    <!-- Settings Modal -->
    {#if showSettings}
      <div
        class="fixed inset-0 bg-black/30 backdrop-blur-sm flex items-center justify-center z-50"
        on:click|self={() => (showSettings = false)}
      >
        <div
          class="bg-surface-strong p-6 rounded-2xl shadow-card border border-soft w-full max-w-md transform transition-all"
        >
          <h2 class="text-xl font-semibold mb-4">Settings</h2>
          <div class="space-y-4">
            <div>
              <label class="block text-sm font-medium text-muted mb-1"
                >Google AI API Key</label
              >
              <input
                type="password"
                bind:value={apiKey}
                placeholder="AIza..."
                class="input"
              />
              <p class="text-xs text-muted mt-1">
                Required for AI parsing features.
              </p>
            </div>
            <div class="flex justify-end space-x-3 pt-4">
              <button
                on:click={() => (showSettings = false)}
                class="btn btn-ghost"
              >
                Cancel
              </button>
              <button
                on:click={saveSettings}
                class="btn btn-accent"
              >
                Save Settings
              </button>
            </div>
          </div>
        </div>
      </div>
    {/if}

    <!-- Main Content -->
    <div
      class="flex-1 min-h-0 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8"
    >
      <!-- Left Panel: Controls -->
      <div class="lg:col-span-1 space-y-6 overflow-y-auto pr-2">
        <!-- Step 1: Select Folder -->
        <div class="bg-surface rounded-2xl p-6 border border-soft shadow-soft">
          <h2
            class="text-lg font-semibold mb-4 flex items-center"
          >
            <span
              class="bg-teal-100 text-teal-700 w-6 h-6 rounded-full flex items-center justify-center text-xs mr-2"
              >1</span
            >
            Select Source
          </h2>
          <button
            on:click={selectFolder}
            disabled={isLoading}
            class="btn btn-primary w-full disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <svg
              xmlns="http://www.w3.org/2000/svg"
              class="h-5 w-5"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"
              />
            </svg>
            <span>Browse Folder</span>
          </button>
          {#if folderPath}
            <div
              class="mt-3 p-3 bg-surface-strong rounded-xl border border-soft"
            >
              <p
                class="text-xs text-muted uppercase tracking-wider font-bold mb-1"
              >
                Selected Path
              </p>
              <p class="text-sm text-ink break-all font-mono">
                {folderPath}
              </p>
            </div>
          {/if}
        </div>

        <!-- Step 2: Actions -->
        {#if localTracks.length > 0}
          <div class="bg-surface rounded-2xl p-6 border border-soft shadow-soft fade-in">
            <h2 class="text-lg font-semibold mb-4 flex items-center">
              <span
                class="bg-teal-100 text-teal-700 w-6 h-6 rounded-full flex items-center justify-center text-xs mr-2"
                >2</span
              >
              Choose Method
            </h2>

            <div class="space-y-4">
              <!-- Template -->
              <!-- Template -->
              <div class="bg-surface-strong rounded-xl p-4 space-y-3 border border-soft">
                <button
                  on:click={generateFromTemplate}
                  disabled={isLoading}
                  class="w-full flex items-center justify-between group disabled:opacity-50"
                >
                  <div class="flex items-center text-ink font-medium">
                    <svg
                      xmlns="http://www.w3.org/2000/svg"
                      class="h-5 w-5 mr-3 text-teal-600"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z"
                      />
                    </svg>
                    <span>Template Pattern</span>
                  </div>
                  <span class="text-xs text-muted group-hover:text-ink"
                    >Fast</span
                  >
                </button>

                <div class="pt-2 border-t border-soft">
                  <label class="text-xs text-muted block mb-1">Format</label>
                  <select
                    bind:value={templateFormat}
                    class="input text-sm"
                  >
                    <option value="Track. Artist - Title"
                      >Track. Artist - Title</option
                    >
                    <option value="Track. Title">Track. Title</option>
                  </select>
                </div>
              </div>

              <!-- AI Parsing -->
              <button
                on:click={parseWithAI}
                disabled={isLoading}
                class="w-full py-3 px-4 bg-gradient-to-r from-teal-700 to-amber-600 text-white rounded-xl font-medium transition-all text-left flex items-center justify-between group disabled:opacity-50 shadow-soft"
              >
                <div class="flex items-center">
                  <svg
                    xmlns="http://www.w3.org/2000/svg"
                    class="h-5 w-5 mr-3 text-white"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M13 10V3L4 14h7v7l9-11h-7z"
                    />
                  </svg>
                  <span>AI Smart Parse</span>
                </div>
                <span class="text-xs text-white/80 bg-white/20 px-2 py-0.5 rounded"
                  >New</span
                >
              </button>

              <!-- Bandcamp / Beatport -->
              <div class="pt-2 border-t border-soft">
                <label class="text-xs text-muted font-medium mb-2 block"
                  >Bandcamp / Beatport Match</label
                >
                <div class="flex space-x-2">
                  <input
                    type="text"
                    bind:value={bandcampUrl}
                    placeholder="Bandcamp or Beatport URL..."
                    disabled={isLoading}
                    class="input text-sm"
                  />
                  <button
                    on:click={fetchAndMatch}
                    disabled={isLoading || !bandcampUrl}
                    class="btn btn-accent p-2 disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    <svg
                      xmlns="http://www.w3.org/2000/svg"
                      class="h-5 w-5"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
                      />
                    </svg>
                  </button>
                </div>
              </div>
            </div>
          </div>
        {/if}
      </div>

      <!-- Right Panel: Results -->
      <div class="lg:col-span-2 flex flex-col h-full min-h-0">
        {#if notification}
          <div
            class="mb-4 p-4 rounded-2xl bg-surface-strong border-l-4 border-teal-500 shadow-soft fade-in flex items-center flex-none"
          >
            {#if isLoading}
              <svg
                class="animate-spin -ml-1 mr-3 h-5 w-5 text-teal-600"
                xmlns="http://www.w3.org/2000/svg"
                fill="none"
                viewBox="0 0 24 24"
              >
                <circle
                  class="opacity-25"
                  cx="12"
                  cy="12"
                  r="10"
                  stroke="currentColor"
                  stroke-width="4"
                ></circle>
                <path
                  class="opacity-75"
                  fill="currentColor"
                  d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                ></path>
              </svg>
            {/if}
            <span class="text-ink">{notification}</span>
          </div>
        {/if}

        {#if processedTracks.length > 0}
          <div
            class="bg-surface rounded-2xl border border-soft shadow-card flex-1 flex flex-col overflow-hidden fade-in min-h-0"
          >
            <div
              class="p-4 border-b border-soft flex justify-between items-center bg-surface-strong flex-none"
            >
              <h2 class="font-semibold">Proposed Changes</h2>
              <span class="text-sm text-muted"
                >{processedTracks.length} tracks</span
              >
            </div>

            <div class="overflow-y-auto flex-1 p-4 space-y-3">
              {#each processedTracks as match, i}
                <div
                  class="group bg-surface-strong rounded-xl p-3 border border-soft hover:border-teal-200 transition-all"
                >
                  <div class="flex items-start space-x-4">
                    <!-- Status Icon -->
                    <div class="mt-1">
                      {#if match.status === "Original"}
                        <div class="w-2 h-2 rounded-full bg-muted"></div>
                      {:else}
                        <div
                          class={`w-2 h-2 rounded-full ${match.confidence > 0.8 ? "bg-green-600" : match.confidence > 0.4 ? "bg-amber-500" : "bg-red-500"}`}
                        ></div>
                      {/if}
                    </div>

                    <div class="flex-1 space-y-2">
                      <!-- Original Name -->
                      <div
                        class="text-xs text-muted font-mono truncate"
                        title={match.originalName}
                      >
                        {match.originalName}
                      </div>

                      <!-- New Name Input -->
                      <div class="relative">
                        <input
                          type="text"
                          value={match.proposedNewName}
                          on:input={(e) => handleProposedNameChange(e, i)}
                          class={`input font-mono text-sm ${getConfidenceColor(match.confidence)}`}
                        />
                      </div>
                    </div>

                    <!-- Status Badge -->
                    <div class="flex flex-col items-end space-y-1 min-w-[80px]">
                      <span
                        class={`chip ${match.confidence > 0.8 ? "chip-success" : match.confidence > 0.4 ? "chip-warning" : "chip-danger"}`}
                      >
                        {match.status}
                      </span>
                      {#if match.status !== "Original"}
                        <span class="text-xs text-muted"
                          >{(match.confidence * 100).toFixed(0)}% match</span
                        >
                      {/if}
                    </div>
                  </div>
                </div>
              {/each}
            </div>

            <div
              class="p-4 border-t border-soft bg-surface-strong flex-none"
            >
              <button
                on:click={renameFiles}
                disabled={isLoading}
                class="btn btn-primary w-full disabled:opacity-50 disabled:cursor-not-allowed"
              >
                <svg
                  xmlns="http://www.w3.org/2000/svg"
                  class="h-5 w-5"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M5 13l4 4L19 7"
                  />
                </svg>
                <span>Apply Rename to {processedTracks.length} Files</span>
              </button>
            </div>
          </div>
        {:else if !isLoading && folderPath}
          <div
            class="flex-1 flex items-center justify-center text-muted border-2 border-dashed border-soft rounded-2xl m-4"
          >
            <div class="text-center">
              <svg
                xmlns="http://www.w3.org/2000/svg"
                class="h-12 w-12 mx-auto mb-3 opacity-50"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"
                />
              </svg>
              <p>Select a method to generate new names</p>
            </div>
          </div>
        {/if}
      </div>
    </div>
  </div>
</main>

<style>
  /* Custom scrollbar for webkit browsers */
  ::-webkit-scrollbar {
    width: 8px;
    height: 8px;
  }
  ::-webkit-scrollbar-track {
    background: #f1e7db;
  }
  ::-webkit-scrollbar-thumb {
    background: #d8c7b4;
    border-radius: 4px;
  }
  ::-webkit-scrollbar-thumb:hover {
    background: #c8b39d;
  }
</style>
